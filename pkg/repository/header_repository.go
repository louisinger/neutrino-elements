package repository

import (
	"context"
	"errors"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/vulpemventures/go-elements/block"
)

var (
	ErrBlockNotFound   = errors.New("block not found")
	ErrNoBlocksHeaders = errors.New("no block headers in repository")
)

type BlockHeaderRepository interface {
	// chain tip returns the best block header in the store
	ChainTip(context.Context) (*block.Header, error)
	GetBlockHeader(context.Context, chainhash.Hash) (*block.Header, error)
	GetBlockHashByHeight(context.Context, uint32) (*chainhash.Hash, error)
	WriteHeaders(context.Context, ...block.Header) error
	// LatestBlockLocator returns the block locator for the latest known tip as root of the locator
	LatestBlockLocator(context.Context) (blockchain.BlockLocator, error)
	HasAllAncestors(context.Context, chainhash.Hash) (bool, error)
}
