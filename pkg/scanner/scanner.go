package scanner

import (
	"context"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcutil/gcs/builder"
	"github.com/btcsuite/btcutil/hdkeychain"
	"github.com/sirupsen/logrus"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/go-elements/payment"
	"github.com/vulpemventures/go-elements/transaction"
	"github.com/vulpemventures/neutrino-elements/pkg/blockservice"
	"github.com/vulpemventures/neutrino-elements/pkg/protocol"
	"github.com/vulpemventures/neutrino-elements/pkg/repository"
)

type Report struct {
	// Transaction is the transaction that includes the item that was found.
	Transaction *transaction.Transaction

	// BlockHash is the block hash of the block that includes the transaction.
	BlockHash   *chainhash.Hash
	BlockHeight uint32

	// the request resolved by the report
	Request *ScanRequest
}

type RestorationResult struct {
	LatestInternalIndex uint32
	LatestExternalIndex uint32
	Reports             []Report
}

type ScannerService interface {
	// Start runs a go-routine in order to handle incoming requests via Watch
	Start() error
	// Stop the scanner
	Stop()
	// Add a new request to the queue
	Watch(...ScanRequestOption) <-chan Report
	Rescan(xpub string) (*RestorationResult, error)
}

type scannerService struct {
	started       bool
	requestsQueue *scanRequestQueue
	filterDB      repository.FilterRepository
	headerDB      repository.BlockHeaderRepository
	genesisHash   *chainhash.Hash
	blockService  blockservice.BlockService
	quitCh        chan struct{}
}

var _ ScannerService = (*scannerService)(nil)

func New(
	filterDB repository.FilterRepository,
	headerDB repository.BlockHeaderRepository,
	blockSvc blockservice.BlockService,
	genesisHash *chainhash.Hash,
) ScannerService {
	return &scannerService{
		requestsQueue: newScanRequestQueue(),
		filterDB:      filterDB,
		headerDB:      headerDB,
		blockService:  blockSvc,
		quitCh:        make(chan struct{}),
		genesisHash:   genesisHash,
	}
}

func (s *scannerService) Start() error {
	if s.started {
		return fmt.Errorf("utxo scanner already started")
	}

	s.quitCh = make(chan struct{}, 1)
	// start the requests manager goroutine
	go s.requestsManager()

	s.started = true
	return nil
}

func (s *scannerService) Stop() {
	s.quitCh <- struct{}{}
	s.started = false
	s.requestsQueue = newScanRequestQueue()
}

func (s *scannerService) Watch(opts ...ScanRequestOption) <-chan Report {
	req := newScanRequest(opts...)
	if req.Out == nil {
		req.Out = make(chan Report)
	}
	s.requestsQueue.enqueue(req)

	return req.Out
}

func (s *scannerService) Rescan(xpub string) (*RestorationResult, error) {
	if !s.started {
		return nil, fmt.Errorf("utxo scanner not started")
	}

	hdNode, err := hdkeychain.NewKeyFromString(xpub)
	if err != nil {
		return nil, err
	}

	external, err := hdNode.Derive(0)
	if err != nil {
		return nil, err
	}

	internal, err := hdNode.Derive(1)
	if err != nil {
		return nil, err
	}

	latestExternalIndex, externalReports, err := s.rescanHdNode(external, 20)
	if err != nil {
		return nil, err
	}

	latestInternalIndex, internalReports, err := s.rescanHdNode(internal, 20)
	if err != nil {
		return nil, err
	}

	return &RestorationResult{
		LatestExternalIndex: latestExternalIndex,
		LatestInternalIndex: latestInternalIndex,
		Reports:             append(externalReports, internalReports...),
	}, nil
}

func (s *scannerService) rescanHdNode(node *hdkeychain.ExtendedKey, gaplimit int) (uint32, []Report, error) {
	gap := 0
	index := uint32(0)

	reports := make([]Report, 0)

	for gap < gaplimit {
		items := make([]WatchItem, gaplimit-gap)
		for i := 0; i < gaplimit-gap; i++ {
			key, err := node.Derive(index)
			if err != nil {
				return 0, nil, errors.New("error deriving key")
			}

			index++

			pubkey, err := key.ECPubKey()
			if err != nil {
				return 0, nil, errors.New("error deriving pubkey")
			}

			script := payment.FromPublicKey(pubkey, s.network(), nil).WitnessScript
			items[i] = NewScriptWatchItem(script)
		}

		channels := make([]<-chan Report, len(items))

		for i, item := range items {
			channels[i] = s.Watch(
				WithWatchItem(item),
				WithStartBlock(0),
			)
		}

		for _, channel := range channels {
			report := <-channel // wait for report
			reports = append(reports, report)
			// TODO handle "not found" report (need WithEndBlock for instance in request)
			// if (error) gap++ else gap = 0
			gap = 0
		}
	}

	return index, reports, nil
}

func (s *scannerService) network() *network.Network {
	switch s.genesisHash.String() {
	case protocol.LiquidGenesisBlockHash:
		return &network.Liquid
	case protocol.LiquidTestnetGenesisBlockHash:
		return &network.Testnet
	case protocol.NigiriGenesisBlockHash:
		return &network.Regtest
	default:
		return nil
	}
}

// requestsManager is responsible to resolve the requests that are waiting for in the queue.
func (s *scannerService) requestsManager() {
	defer close(s.quitCh)

	for {
		s.requestsQueue.cond.L.Lock()
		for s.requestsQueue.isEmpty() {
			logrus.Debug("scanner queue is empty, waiting for new requests")
			s.requestsQueue.cond.Wait() // wait for new requests

			// check if we should quit the routine
			select {
			case <-s.quitCh:
				s.requestsQueue.cond.L.Unlock()
				return
			default:
			}
		}
		s.requestsQueue.cond.L.Unlock()

		// get the next request without removing it from the queue
		nextRequest := s.requestsQueue.peek()
		err := s.requestWorker(nextRequest.StartHeight)
		if err != nil {
			logrus.Errorf("error while scanning: %v", err)
		}

		// check if we should quit the routine
		select {
		case <-s.quitCh:
			return
		default:
			continue
		}

	}
}

// will check if any blocks has the requested item
// if yes, will extract the transactions that match the requests' watchItems
// TODO handle properly errors (enqueue the unresolved requests ??)
func (s *scannerService) requestWorker(startHeight uint32) error {
	nextBatch := make([]*ScanRequest, 0)
	nextHeight := startHeight

	chainTip, err := s.headerDB.ChainTip(context.Background())
	if err != nil {
		return err
	}

	for nextHeight <= chainTip.Height {
		// append all the requests with start height = nextHeight
		nextBatch = append(nextBatch, s.requestsQueue.dequeueAtHeight(nextHeight)...)

		itemsBytes := make([][]byte, 0)
		for _, req := range nextBatch {
			itemsBytes = append(itemsBytes, req.Item.Bytes())
		}

		// get the block hash for height
		var blockHash *chainhash.Hash
		if nextHeight == 0 {
			blockHash = s.genesisHash
		} else {
			blockHash, err = s.headerDB.GetBlockHashByHeight(context.Background(), nextHeight)
			if err != nil {
				return err
			}
		}

		// check with filterDB if the block has one of the items
		matched, err := s.blockFilterMatches(itemsBytes, blockHash)
		if err != nil {
			return err
		}

		if matched {
			reports, remainReqs, err := s.extractBlockMatches(blockHash, nextBatch)
			if err != nil {
				return err
			}

			for _, report := range reports {
				// send the report to the output channel
				report.Request.Out <- report

				// if the request is persistent, the scanner will keep watching the item at the next block height
				if report.Request.IsPersistent {
					s.Watch(
						WithStartBlock(report.BlockHeight+1),
						WithWatchItem(report.Request.Item),
						WithReportsChan(report.Request.Out),
						WithPersistentWatch(),
					)
				}
			}

			// if some requests remain, put them back in the next batch
			// this will remove the resolved requests from the batch
			nextBatch = remainReqs
		}

		// increment the height to scan
		// if nothing was found, we can just continue with same batch and next height
		nextHeight++

		chainTip, err = s.headerDB.ChainTip(context.Background())
		if err != nil {
			return err
		}
	}

	// enqueue the remaining requests
	for _, req := range nextBatch {
		s.requestsQueue.enqueue(req)
	}

	return nil
}

func (s *scannerService) blockFilterMatches(items [][]byte, blockHash *chainhash.Hash) (bool, error) {
	filterToFetchKey := repository.FilterKey{
		BlockHash:  blockHash.CloneBytes(),
		FilterType: repository.RegularFilter,
	}

	filter, err := s.filterDB.GetFilter(context.Background(), filterToFetchKey)
	if err != nil {
		if err == repository.ErrFilterNotFound {
			return false, nil
		}
		return false, err
	}

	gcsFilter, err := filter.GcsFilter()
	if err != nil {
		return false, err
	}

	key := builder.DeriveKey(blockHash)
	matched, err := gcsFilter.MatchAny(key, items)
	if err != nil {
		return false, err
	}

	return matched, nil
}

func (s *scannerService) extractBlockMatches(blockHash *chainhash.Hash, requests []*ScanRequest) ([]Report, []*ScanRequest, error) {
	block, err := s.blockService.GetBlock(blockHash)
	if err != nil {
		if err == blockservice.ErrorBlockNotFound {
			return nil, requests, nil // skip requests if block svc is not able to find the block
		}

		return nil, nil, err
	}

	results := make([]Report, 0)

	remainRequests := make([]*ScanRequest, 0)

	for _, req := range requests {
		reqMatchedAtLeastOneTime := false
		for _, tx := range block.TransactionsData.Transactions {
			if req.Item.Match(tx) {
				reqMatchedAtLeastOneTime = true
				results = append(results, Report{
					Transaction: tx,
					BlockHash:   blockHash,
					BlockHeight: block.Header.Height,
					Request:     req,
				})
			}
		}

		if !reqMatchedAtLeastOneTime {
			remainRequests = append(remainRequests, req)
		}
	}

	return results, remainRequests, nil
}
