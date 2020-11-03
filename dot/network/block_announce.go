// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package network

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/scale"

	libp2pnetwork "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

type blockAnnounceData struct {
	validated bool                  // set to true if a handshake has been received and validated, false otherwise
	msg       *BlockAnnounceMessage // if this node is the sender of the BlockAnnounce, this is set, otherwise, it's nil
}

func blockAnnounceHandshakeDecoder(in []byte) (Message, error) {
	r := &bytes.Buffer{}
	_, err := r.Write(in)
	if err != nil {
		return nil, err
	}

	hs := new(BlockAnnounceHandshake)
	return hs, hs.Decode(r)
}

// BlockAnnounceHandshake is exchanged by nodes that are beginning the BlockAnnounce protocol
type BlockAnnounceHandshake struct {
	Roles           byte
	BestBlockNumber uint64
	BestBlockHash   common.Hash
	GenesisHash     common.Hash
}

// String formats a BlockAnnounceHandshake as a string
func (hs *BlockAnnounceHandshake) String() string {
	return fmt.Sprintf("BlockAnnounceHandshake Roles=%d BestBlockNumber=%d BestBlockHash=%s GenesisHash=%s",
		hs.Roles,
		hs.BestBlockNumber,
		hs.BestBlockHash,
		hs.GenesisHash)
}

// Encode encodes a BlockAnnounceHandshake message using SCALE
func (hs *BlockAnnounceHandshake) Encode() ([]byte, error) {
	return scale.Encode(hs)
}

// Decode the message into a BlockAnnounceHandshake
func (hs *BlockAnnounceHandshake) Decode(r io.Reader) error {
	sd := scale.Decoder{Reader: r}
	_, err := sd.Decode(hs)
	return err
}

// Type ...
func (hs *BlockAnnounceHandshake) Type() int {
	return -1
}

// IDString ...
func (hs *BlockAnnounceHandshake) IDString() string {
	return ""
}

func (s *Service) getBlockAnnounceHandshake() (*BlockAnnounceHandshake, error) {
	latestBlock, err := s.blockState.BestBlockHeader()
	if err != nil {
		return nil, err
	}

	return &BlockAnnounceHandshake{
		Roles:           s.cfg.Roles,
		BestBlockNumber: latestBlock.Number.Uint64(),
		BestBlockHash:   latestBlock.Hash(),
		GenesisHash:     s.blockState.GenesisHash(),
	}, nil
}

func (s *Service) validateBlockAnnounceHandshake(hs *BlockAnnounceHandshake) error {
	if hs.GenesisHash != s.blockState.GenesisHash() {
		return errors.New("genesis hash mismatch")
	}

	return nil
}

// handleBlockAnnounceStream handles streams with the <protocol-id>/block-announces/1 protocol ID
func (s *Service) handleBlockAnnounceStream(stream libp2pnetwork.Stream) {
	conn := stream.Conn()
	if conn == nil {
		logger.Error("Failed to get connection from stream")
		return
	}

	peer := conn.RemotePeer()

	// we haven't received a handshake yet from the peer, get handshake first
	if hsData, has := s.blockAnnounceHandshakes[peer]; !has {
		s.readStream(stream, peer, blockAnnounceHandshakeDecoder, s.handleBlockAnnounceMessage)
		return
	} else if !hsData.validated {
		// we received a handshake, but it was invalid
		return
	}

	s.readStream(stream, peer, decodeMessageBytes, s.handleBlockAnnounceMessage)
}

// handleBlockAnnounceMessage handles BlockAnnounce messages
// if some more blocks are required to sync the announced block, the node will open a sync stream
// with its peer and send a BlockRequest message
func (s *Service) handleBlockAnnounceMessage(peer peer.ID, msg Message) {
	if hs, ok := msg.(*BlockAnnounceHandshake); ok {
		// if we are the receiver and haven't received the handshake already, validate it
		if _, has := s.blockAnnounceHandshakes[peer]; !has {
			err := s.validateBlockAnnounceHandshake(hs)
			if err != nil {
				logger.Error("failed to validate BlockAnnounceHandshake", "peer", peer, "error", err)
				s.blockAnnounceHandshakes[peer] = &blockAnnounceData{
					validated: false,
				}
				return
			}

			s.blockAnnounceHandshakes[peer] = &blockAnnounceData{
				validated: true,
			}
			return
		}

		// if we are the initiator and haven't received the handshake already, validate it
		if hsData, has := s.blockAnnounceHandshakes[peer]; has && !hsData.validated {
			err := s.validateBlockAnnounceHandshake(hs)
			if err != nil {
				logger.Error("failed to validate BlockAnnounceHandshake", "peer", peer, "error", err)
				delete(s.blockAnnounceHandshakes, peer)
			}
			return
		}

		// if we are the initiator, send the BlockAnnounce
		if hsData, has := s.blockAnnounceHandshakes[peer]; has && hsData.msg != nil {
			err := s.host.send(peer, blockAnnounceID, s.blockAnnounceHandshakes[peer].msg)
			if err != nil {
				logger.Error("failed to send BlockAnnounceMessage", "peer", peer, "error", err)
			}
			return
		}

		// otherwise, send back a handshake
		resp, err := s.getBlockAnnounceHandshake()
		if err != nil {
			logger.Error("failed to get BlockAnnounceHandshake", "error", err)
			return
		}

		err = s.host.send(peer, blockAnnounceID, resp)
		if err != nil {
			logger.Error("failed to send BlockAnnounceHandshake", "peer", peer, "error", err)
		}
	}

	if an, ok := msg.(*BlockAnnounceMessage); ok {
		req := s.syncer.HandleBlockAnnounce(an)
		if req != nil {
			s.requestTracker.addRequestedBlockID(req.ID)
			err := s.host.send(peer, syncID, req)
			if err != nil {
				logger.Error("failed to send BlockRequest message", "peer", peer)
			}
		}
	}
}
