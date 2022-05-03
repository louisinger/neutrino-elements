package testutils

import (
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/tdex-network/tdex-daemon/pkg/explorer/esplora"
	"github.com/vulpemventures/neutrino-elements/pkg/blockservice"
	"github.com/vulpemventures/neutrino-elements/pkg/node"
	"github.com/vulpemventures/neutrino-elements/pkg/protocol"
	"github.com/vulpemventures/neutrino-elements/pkg/repository/inmemory"
	"github.com/vulpemventures/neutrino-elements/pkg/scanner"
)

var repoFilter = inmemory.NewFilterInmemory()
var repoHeader = inmemory.NewHeaderInmemory()

func Faucet(addr string) (string, error) {
	svc, err := esplora.NewService("http://127.0.0.1:3001", 5000)
	if err != nil {
		return "", err
	}

	return svc.Faucet(addr, 1, "5ac9f65c0efcc4775e0baec4ec03abdde22473cd3cf33c0419ca290e0751b225")
}

func NigiriTestServices() (node.NodeService, scanner.ScannerService) {
	n, err := node.New(node.NodeConfig{
		Network:        "nigiri",
		UserAgent:      "neutrino-elements:test",
		FiltersDB:      repoFilter,
		BlockHeadersDB: repoHeader,
	})

	if err != nil {
		panic(err)
	}

	err = n.Start("localhost:18886")
	if err != nil {
		panic(err)
	}

	time.Sleep(time.Second * 3) // wait for the node sync the first headers if the repo is empty

	blockSvc := blockservice.NewEsploraBlockService("http://localhost:3001")
	genesisBlockHash := protocol.GetCheckpoints(protocol.MagicNigiri)[0]
	h, err := chainhash.NewHashFromStr(genesisBlockHash)
	if err != nil {
		panic(err)
	}
	s := scanner.New(repoFilter, repoHeader, blockSvc, h)

	err = s.Start()
	if err != nil {
		panic(err)
	}

	return n, s
}
