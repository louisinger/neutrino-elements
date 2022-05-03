package rescan_test

import (
	"strconv"
	"testing"

	"github.com/tdex-network/tdex-daemon/pkg/wallet"
	"github.com/vulpemventures/go-elements/network"
	"github.com/vulpemventures/neutrino-elements/pkg/rescan"
	"github.com/vulpemventures/neutrino-elements/pkg/testutils"
)

// Test Rescan Options

var n, s = testutils.NigiriTestServices()

func TestRescanXPub(t *testing.T) {
	randomMnemonic, err := wallet.NewMnemonic(wallet.NewMnemonicOpts{
		EntropySize: 128,
	})
	if err != nil {
		t.Fatal(err)
	}

	birth, _ := n.GetChainTip()

	w, err := wallet.NewWalletFromMnemonic(wallet.NewWalletFromMnemonicOpts{
		SigningMnemonic:  randomMnemonic,
		BlindingMnemonic: randomMnemonic,
	})

	if err != nil {
		t.Fatal(err)
	}

	const NUMBER_OF_ADDRESSES = 22
	const NUMBER_OF_CHANGE_ADDRESSES = 1

	xpub, err := w.ExtendedPublicKey(wallet.ExtendedKeyOpts{
		Account: 0,
	})

	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < NUMBER_OF_ADDRESSES; i++ {
		a, _, err := w.DeriveConfidentialAddress(wallet.DeriveConfidentialAddressOpts{
			DerivationPath: "0'/0/" + strconv.Itoa(i),
			Network:        &network.Regtest,
		})
		if err != nil {
			t.Fatal(err)
		}

		if i == 7 || i == 15 {
			_, err := testutils.Faucet(a)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	for i := 0; i < NUMBER_OF_CHANGE_ADDRESSES; i++ {
		a, _, err := w.DeriveConfidentialAddress(wallet.DeriveConfidentialAddressOpts{
			DerivationPath: "0'/1/" + strconv.Itoa(i),
			Network:        &network.Regtest,
		})
		if err != nil {
			t.Fatal(err)
		}

		if i == 3 || i == 7 || i == NUMBER_OF_CHANGE_ADDRESSES {
			_, err := testutils.Faucet(a)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	tip, _ := n.GetChainTip()

	// rescan XPUB
	reportsChan, resultChan, err := rescan.RescanXPub(xpub, rescan.WithScanner(s), rescan.WithStartHeight(birth.Height), rescan.WithEndHeight(tip.Height))
	if err != nil {
		t.Fatal(err)
	}

	numberOfReports := 0
	var result *rescan.RescanXPubResult = nil
	for {
		select {
		case <-reportsChan:
			numberOfReports++
		case r := <-resultChan:
			result = r
			goto FinalTest
		}
	}

FinalTest:
	if numberOfReports != NUMBER_OF_ADDRESSES+NUMBER_OF_CHANGE_ADDRESSES {
		t.Fatalf("number of reports is not correct: %d", numberOfReports)
	}

	if result.LatestInternalIndex != NUMBER_OF_CHANGE_ADDRESSES {
		t.Fatalf("latest internal index is not correct: %d (expected %d)", result.LatestInternalIndex, NUMBER_OF_CHANGE_ADDRESSES)
	}

	if result.LatestExternalIndex != NUMBER_OF_ADDRESSES {
		t.Fatalf("latest external index is not correct: %d (expected %d)", result.LatestExternalIndex, NUMBER_OF_ADDRESSES)
	}
}
