package state

import (
	"encoding/json"
	"math/big"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/core/blocktree"
	"github.com/ChainSafe/gossamer/core/types"
	"github.com/ChainSafe/gossamer/polkadb"
)

type blockState struct {
	bt *blocktree.BlockTree
	db *polkadb.BlockDB
}

func NewBlockState() *blockState {
	return &blockState{
		bt: &blocktree.BlockTree{},
		db: &polkadb.BlockDB{},
	}
}

var (
	// Data prefixes
	headerPrefix    = []byte("hdr") // headerPrefix + hash -> header
	blockDataPrefix = []byte("hsh") // blockDataPrefix + hash -> blockData
)

// headerKey = headerPrefix + hash
func headerKey(hash common.Hash) []byte {
	return append(headerPrefix, hash.ToBytes()...)
}

// blockDataKey = blockDataPrefix + hash
func blockDataKey(hash common.Hash) []byte {
	return append(blockDataPrefix, hash.ToBytes()...)
}

func (bs *blockState) GetHeader(hash common.Hash) (types.BlockHeaderWithHash, error) {
	var result types.BlockHeaderWithHash

	data, err := bs.db.Db.Get(headerKey(hash))
	if err != nil {
		return types.BlockHeaderWithHash{}, err
	}

	err = json.Unmarshal(data, &result)

	return result, err
}

func (bs *blockState) GetBlockData(hash common.Hash) (types.BlockData, error) {
	var result types.BlockData

	data, err := bs.db.Db.Get(blockDataKey(hash))
	if err != nil {
		return types.BlockData{}, err
	}

	err = json.Unmarshal(data, &result)

	return result, err
}

func (bs *blockState) GetLatestBlock() types.BlockHeaderWithHash {
	// Can't do yet
	return types.BlockHeaderWithHash{}

}

func (bs *blockState) GetBlockByHash(hash common.Hash) (types.Block, error) {
	header, err := bs.GetHeader(hash)
	if err != nil {
		return types.Block{}, nil
	}
	blockData, err := bs.GetBlockData(hash)
	if err != nil {
		return types.Block{}, nil
	}
	blockBody := blockData.Body
	return types.Block{Header: header, Body: *blockBody}, nil
}

func (bs *blockState) GetBlockByNumber(n *big.Int) types.Block {
	// Can't do yet
	return types.Block{}
}

func (bs *blockState) SetHeader(header types.BlockHeaderWithHash) error {
	hash := header.Hash

	// Write the encoded header
	bh, err := json.Marshal(header)
	if err != nil {
		return err
	}

	err = bs.db.Db.Put(headerKey(hash), bh)
	return err
}

func (bs *blockState) SetBlockData(hash common.Hash, blockData types.BlockData) error {
	// Write the encoded header
	bh, err := json.Marshal(blockData)
	if err != nil {
		return err
	}

	err = bs.db.Db.Put(blockDataKey(hash), bh)
	return err
}