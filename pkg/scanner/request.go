package scanner

import "errors"

var (
	ErrEndWithoutMatch = errors.New("the watch item does not match any transaction in the block height range specified")
)

type ScanRequest struct {
	StartHeight  uint32    // nil means scan from genesis block
	EndHeight    uint32    // nil means scan until we math a tx (wait for new blocks if needed)
	Item         WatchItem // item to watch
	IsPersistent bool      // if true, another req will be watched if a report has been found
	Out          chan Report
}

type ScanRequestOption func(req *ScanRequest)

func WithWatchItem(item WatchItem) ScanRequestOption {
	return func(req *ScanRequest) {
		req.Item = item
	}
}

func WithStartBlock(blockHeight uint32) ScanRequestOption {
	return func(req *ScanRequest) {
		req.StartHeight = blockHeight
	}
}

func WithEndBlock(blockHeight uint32) ScanRequestOption {
	return func(req *ScanRequest) {
		req.StartHeight = blockHeight
	}
}

func WithPersistentWatch() ScanRequestOption {
	return func(req *ScanRequest) {
		req.IsPersistent = true
	}
}

func WithReportsChan(reportsChan chan Report) ScanRequestOption {
	return func(req *ScanRequest) {
		req.Out = reportsChan
	}
}

func newScanRequest(options ...ScanRequestOption) *ScanRequest {
	req := &ScanRequest{}
	for _, option := range options {
		option(req)
	}
	return req
}
