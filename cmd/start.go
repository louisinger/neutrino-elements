package main

import (
	"os"
	"os/signal"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"github.com/vulpemventures/neutrino-elements/pkg/blockservice"
	"github.com/vulpemventures/neutrino-elements/pkg/node"
	"github.com/vulpemventures/neutrino-elements/pkg/peer"
	"github.com/vulpemventures/neutrino-elements/pkg/protocol"
	"github.com/vulpemventures/neutrino-elements/pkg/scanner"
)

func startAction(state *State) cli.ActionFunc {
	return func(c *cli.Context) error {
		peers := c.StringSlice("connect")
		if len(peers) == 0 {
			return cli.Exit("connect must be specified", 1)
		}

		addr := c.String("address")
		net := c.String("network")
		if net == "" {
			net = "nigiri"
		}

		logrus.Infof("connect to network: %s (magic: %x)", net, protocol.Networks[net])

		// Create a new peer node.
		node, err := node.New(node.NodeConfig{
			Network:        net,
			UserAgent:      "neutrino-elements:0.0.1",
			FiltersDB:      state.filtersDB,
			BlockHeadersDB: state.blockHeadersDB,
		})
		if err != nil {
			panic(err)
		}

		err = node.Start(peers[0]) // regtest
		if err != nil {
			panic(err)
		}

		if len(peers) > 1 {
			// Connect to additional peers.
			for _, p := range peers[1:] {
				pTcp, err := peer.NewPeerTCP(p)
				if err != nil {
					panic(err)
				}

				node.AddOutboundPeer(pTcp)
			}
		}

		magic := protocol.MagicNigiri
		if net == "testnet" {
			magic = protocol.MagicLiquidTestnet
		}

		genesisBlockHash := protocol.GetCheckpoints(magic)[0]
		h, err := chainhash.NewHashFromStr(genesisBlockHash)
		if err != nil {
			panic(err)
		}

		esploraURL := "http://localhost:3001"
		if net == "testnet" {
			esploraURL = "https://liquid-testnet.sevenlabs.dev/"
		}

		blockSvc := blockservice.NewEsploraBlockService(esploraURL)
		scanSvc := scanner.New(state.filtersDB, state.blockHeadersDB, blockSvc, h)
		reportCh, err := scanSvc.Start()
		if err != nil {
			panic(err)
		}

		go func() {
			for report := range reportCh {
				logrus.Infof("SCAN RESOLVE: %+v", report.Transaction.TxHash())
			}
		}()

		// we'll watch if this address receives fund
		watchItem, err := scanner.NewScriptWatchItemFromAddress(addr)
		if err != nil {
			panic(err)
		}

		time.Sleep(time.Second * 3)
		scanSvc.Watch(
			scanner.WithStartBlock(0),
			scanner.WithWatchItem(watchItem),
			scanner.WithPersistentWatch(),
		)
		if err != nil {
			panic(err)
		}

		signalQuit := make(chan os.Signal, 1)
		signal.Notify(signalQuit, os.Interrupt)
		<-signalQuit
		node.Stop()
		scanSvc.Stop()
		return nil
	}
}
