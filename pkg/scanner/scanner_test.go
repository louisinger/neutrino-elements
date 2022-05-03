package scanner_test

import (
	"testing"

	"github.com/vulpemventures/neutrino-elements/pkg/scanner"
	"github.com/vulpemventures/neutrino-elements/pkg/testutils"
)

func TestWatch(t *testing.T) {
	const address = "el1qq0mjw2fwsc20vr4q2ypq9w7dslg6436zaahl083qehyghv7td3wnaawhrpxphtjlh4xjwm6mu29tp9uczkl8cxfyatqc3vgms"

	n, s := testutils.NigiriTestServices()
	defer s.Stop()
	defer n.Stop()

	watchItem, err := scanner.NewScriptWatchItemFromAddress(address)
	if err != nil {
		t.Fatal(err)
	}

	tip, err := n.GetChainTip()
	if err != nil {
		t.Fatal(err)
	}

	reportCh, err := s.Watch(scanner.WithStartBlock(0), scanner.WithEndBlock(tip.Height), scanner.WithWatchItem(watchItem))
	if err != nil {
		t.Fatal(err)
	}

	txid, err := testutils.Faucet(address)
	if err != nil {
		t.Fatal(err)
	}

	nextReport := <-reportCh

	if nextReport.Resolved() {
		if nextReport.Transaction.TxHash().String() != txid {
			t.Fatalf("expected txid %s, got %s", txid, nextReport.Transaction.TxHash().String())
		}
	} else {
		t.Error(nextReport.Error)
	}
}

func TestWatchPersistent(t *testing.T) {
	const address = "el1qqfs4ecf5427tyshnsq0x3jy3ad2tqfn03x3fqmxtyn2ycuvmk98urxmh9cdmr5zcqfs42l6a3kpyrk6pkxjx7yuvqsnuuckhp"

	n, s := testutils.NigiriTestServices()
	defer s.Stop()
	defer n.Stop()

	watchItem, err := scanner.NewScriptWatchItemFromAddress(address)
	if err != nil {
		t.Fatal(err)
	}

	tip, err := n.GetChainTip()
	if err != nil {
		t.Fatal(err)
	}

	reportCh, err := s.Watch(scanner.WithStartBlock(tip.Height+1), scanner.WithWatchItem(watchItem), scanner.WithPersistentWatch())
	if err != nil {
		t.Fatal(err)
	}

	txid, err := testutils.Faucet(address)
	if err != nil {
		t.Fatal(err)
	}

	nextReport := <-reportCh

	if nextReport.Resolved() {
		if nextReport.Transaction.TxHash().String() != txid {
			t.Fatalf("expected txid %s, got %s", txid, nextReport.Transaction.TxHash().String())
		}
	} else {
		t.Fatal(nextReport.Error)
	}

	// we test if the watch is persistent by sending a new transaction
	txid, err = testutils.Faucet(address)
	if err != nil {
		t.Fatal(err)
	}

	nextReport = <-reportCh

	if nextReport.Transaction.TxHash().String() != txid {
		t.Fatalf("expected txid %s, got %s", txid, nextReport.Transaction.TxHash().String())
	}
}
