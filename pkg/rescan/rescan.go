package rescan

import (
	"errors"

	"github.com/btcsuite/btcutil/hdkeychain"
	"github.com/vulpemventures/go-elements/payment"
	"github.com/vulpemventures/neutrino-elements/pkg/scanner"
)

var (
	ErrRescanAlreadyStarted = errors.New("rescan routine is running")
)

type RescanResult struct {
	LatestIndex uint32
}

const defaultBIP32GapLimit = 20

type Rescan struct {
	getNextWatchItem func(index uint32) (scanner.WatchItem, error)
	gaplimit         int
	startHeight      uint32
	endHeight        uint32
	scanner          scanner.ScannerService

	started bool
	quit    chan interface{}
	reports chan scanner.Report
	result  chan RescanResult
}

type RescanOption func(rescan *Rescan)

func WithScanner(svc scanner.ScannerService) RescanOption {
	return func(rescan *Rescan) {
		rescan.scanner = svc
	}
}

func WithEndHeight(height uint32) RescanOption {
	return func(rescan *Rescan) {
		rescan.endHeight = height
	}
}

func WithStartHeight(height uint32) RescanOption {
	return func(rescan *Rescan) {
		rescan.startHeight = height
	}
}

func WithRescanP2WPKH(xpub *hdkeychain.ExtendedKey) RescanOption {
	return func(rescan *Rescan) {
		rescan.getNextWatchItem = func(index uint32) (scanner.WatchItem, error) {
			key, err := xpub.Derive(index)
			if err != nil {
				return nil, err
			}

			pubkey, err := key.ECPubKey()
			if err != nil {
				return nil, err
			}

			script := payment.FromPublicKey(pubkey, rescan.scanner.Network(), nil).Script
			return scanner.NewScriptWatchItem(script), nil
		}
	}
}

func defaultRescanOption() RescanOption {
	return func(rescan *Rescan) {
		rescan.gaplimit = defaultBIP32GapLimit
	}
}

func (r *Rescan) validate() error {
	if r.scanner == nil {
		return errors.New("recscan object needs scanner")
	}

	if r.getNextWatchItem == nil {
		return errors.New("recscan object needs getNextWatchItem")
	}

	return nil
}

func NewRescan(options ...RescanOption) (*Rescan, error) {
	rescan := &Rescan{}
	defaultRescanOption()(rescan)
	for _, option := range options {
		option(rescan)
	}
	return rescan, rescan.validate()
}

func (r *Rescan) Start() (<-chan scanner.Report, <-chan RescanResult, error) {
	if r.started {
		return nil, nil, ErrRescanAlreadyStarted
	}
	r.started = true

	r.quit = make(chan interface{}, 1)
	r.reports = make(chan scanner.Report)
	r.result = make(chan RescanResult, 1)

	go r.rescanBIP32()
	return r.reports, r.result, nil
}

func (r *Rescan) rescanBIP32() {
	gap := 0
	index := uint32(0)

	for gap < r.gaplimit {
		items := make([]scanner.WatchItem, 0)
		for i := 0; i < r.gaplimit-gap; i++ {
			nextItemToWatch, _ := r.getNextWatchItem(uint32(i) + index)
			if nextItemToWatch == nil {
				continue
			}

			items = append(items, nextItemToWatch)
		}

		channels := make([]chan scanner.Report, len(items))

		for i, item := range items {
			channels[i] = make(chan scanner.Report, 1)
			defer close(channels[i])

			_, err := r.scanner.Watch(
				scanner.WithReportsChan(channels[i]),
				scanner.WithWatchItem(item),
				scanner.WithStartBlock(r.startHeight),
				scanner.WithEndBlock(r.endHeight),
			)
			if err != nil {
				channels[i] = nil
			}
		}

		for _, channel := range channels {
			if channel == nil {
				continue // skip errored address generation
			}
			select {
			case report := <-channel:
				index++
				if report.Resolved() {
					r.reports <- report
					gap = 0
				} else {
					gap++ // if the watchItem does not match any tx, increase gap
				}
			case <-r.quit:
				goto End
			}
		}
	}

End:
	r.result <- RescanResult{LatestIndex: index}
	close(r.reports)
	close(r.result)
	close(r.quit)

}

func (r *Rescan) Stop() {
	if !r.started {
		return
	}
	r.quit <- struct{}{}
}

type RescanXPubResult struct {
	LatestExternalIndex uint32
	LatestInternalIndex uint32
}

// RescanXPub aims to scan the chain for a given BIP32-account-like xpub and returns the latest index of the external and internal derivation path.
func RescanXPub(xpubBase64 string, options ...RescanOption) (<-chan scanner.Report, <-chan *RescanXPubResult, error) {
	xpub, err := hdkeychain.NewKeyFromString(xpubBase64)
	if err != nil {
		return nil, nil, err
	}

	externalKey, err := xpub.Derive(0)
	if err != nil {
		return nil, nil, err
	}

	internalKey, err := xpub.Derive(1)
	if err != nil {
		return nil, nil, err
	}

	externalRescan, err := NewRescan(append(options, WithRescanP2WPKH(externalKey))...)
	if err != nil {
		return nil, nil, err
	}

	internalRescan, err := NewRescan(append(options, WithRescanP2WPKH(internalKey))...)
	if err != nil {
		return nil, nil, err
	}

	reports := make(chan scanner.Report)
	result := make(chan *RescanXPubResult, 1)

	go func() {
		finalResult := &RescanXPubResult{}
		externalReports, externalResult, err := externalRescan.Start()
		if err != nil {
			goto InternalRestore
		}

		for r := range externalReports {
			reports <- r
		}
		finalResult.LatestExternalIndex = (<-externalResult).LatestIndex

	InternalRestore:
		internalReports, internalResult, err := internalRescan.Start()
		if err != nil {
			goto End
		}

		for r := range internalReports {
			reports <- r
		}
		finalResult.LatestInternalIndex = (<-internalResult).LatestIndex
	End:
		externalRescan.Stop()
		internalRescan.Stop()
		result <- finalResult
		close(reports)
		close(result)
	}()

	return reports, result, nil
}
