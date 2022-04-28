package scanner

type ScanRequest struct {
	StartHeight  uint32    // nil means scan from genesis block
	Item         WatchItem // items to watch
	IsPersistent bool      // if true, the request will be re-added with StartHeight = StartHeiht + 1
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
