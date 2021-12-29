package node

import (
	"bytes"
	"fmt"
	"io"
	"net"

	"github.com/sirupsen/logrus"
	"github.com/vulpemventures/go-elements/block"
	"github.com/vulpemventures/neutrino-elements/pkg/binary"
	"github.com/vulpemventures/neutrino-elements/pkg/protocol"
	"github.com/vulpemventures/neutrino-elements/pkg/repository"
	"github.com/vulpemventures/neutrino-elements/pkg/repository/inmemory"
)

// PeerID is peer IP address.
type PeerID string

// Node implements a Bitcoin node.
type Node struct {
	Network      string
	NetworkMagic protocol.Magic
	Peers        map[PeerID]*Peer
	PingCh       chan peerPing
	PongCh       chan uint64
	DisconCh     chan PeerID
	UserAgent    string

	blockHeadersCh chan block.Header
	filtersDb      repository.FilterRepository
	blockHeadersDb repository.BlockHeaderRepository
}

// New returns a new Node.
func New(network, userAgent string) (*Node, error) {
	networkMagic, ok := protocol.Networks[network]
	if !ok {
		return nil, fmt.Errorf("unsupported network %s", network)
	}

	return &Node{
		Network:      network,
		NetworkMagic: networkMagic,
		Peers:        make(map[PeerID]*Peer),
		PingCh:       make(chan peerPing),
		DisconCh:     make(chan PeerID),
		PongCh:       make(chan uint64),
		UserAgent:    userAgent,

		blockHeadersCh: make(chan block.Header),
		filtersDb:      inmemory.NewFilterInmemory(),
		blockHeadersDb: inmemory.NewHeaderInmemory(),
	}, nil
}

// Run starts a node.
func (no Node) Run(nodeAddr string) error {
	peerAddr, err := ParseNodeAddr(nodeAddr)
	if err != nil {
		return err
	}

	version, err := no.createNodeVersionMsg(peerAddr)
	if err != nil {
		return err
	}

	conn, err := net.Dial("tcp", nodeAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	err = no.sendMessage(conn, version)
	if err != nil {
		return err
	}

	go no.monitorPeers()
	go no.monitorBlockHeaders()

	tmp := make([]byte, protocol.MsgHeaderLength)

Loop:
	for {
		n, err := conn.Read(tmp)
		if err != nil {
			if err != io.EOF {
				return err
			}
			logrus.Errorf(err.Error())
			break Loop
		}

		var msgHeader protocol.MessageHeader
		if err := binary.NewDecoder(bytes.NewReader(tmp[:n])).Decode(&msgHeader); err != nil {
			logrus.Errorf("invalid header: %+v", err)
			continue
		}

		if err := msgHeader.Validate(); err != nil {
			logrus.Error(err)
			continue
		}

		logrus.Debugf("received message: %s", msgHeader.Command)

		switch msgHeader.CommandString() {
		case "version":
			if err := no.handleVersion(&msgHeader, conn); err != nil {
				logrus.Errorf("failed to handle 'version': %+v", err)
				continue
			}
		case "verack":
			if err := no.handleVerack(&msgHeader, conn); err != nil {
				logrus.Errorf("failed to handle 'verack': %+v", err)
				continue
			}
		case "ping":
			if err := no.handlePing(&msgHeader, conn); err != nil {
				logrus.Errorf("failed to handle 'ping': %+v", err)
				continue
			}
		case "pong":
			if err := no.handlePong(&msgHeader, conn); err != nil {
				logrus.Errorf("failed to handle 'pong': %+v", err)
				continue
			}
		case "inv":
			if err := no.handleInv(&msgHeader, conn); err != nil {
				logrus.Errorf("failed to handle 'inv': %+v", err)
				continue
			}
		case "tx":
			if err := no.handleTx(&msgHeader, conn); err != nil {
				logrus.Errorf("failed to handle 'tx': %+v", err)
				continue
			}
		case "block":
			if err := no.handleBlock(&msgHeader, conn); err != nil {
				logrus.Errorf("failed to handle 'block': %+v", err)
				continue
			}
		case "sendcmpct":
			if err := no.handleSendCmpct(&msgHeader, conn); err != nil {
				logrus.Errorf("failed to handle 'sendcmpct': %+v", err)
				continue
			}
		case "getheaders":
			if err := no.handleGetHeaders(&msgHeader, conn); err != nil {
				logrus.Errorf("failed to handle 'getheaders': %+v", err)
				continue
			}
		}
	}

	return nil
}

func (no Node) createNodeVersionMsg(peerAddr *Addr) (*protocol.Message, error) {
	return protocol.NewVersionMsg(
		no.Network,
		no.UserAgent,
		peerAddr.IP,
		peerAddr.Port,
	)
}

func (no *Node) sendMessage(conn io.Writer, msg *protocol.Message) error {
	msgSerialized, err := binary.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = conn.Write(msgSerialized)
	return err
}

func (no Node) disconnectPeer(peerID PeerID) {
	logrus.Debugf("disconnecting peer %s", peerID)

	peer := no.Peers[peerID]
	if peer == nil {
		return
	}

	peer.Connection.Close()
	delete(no.Peers, peerID)
}

func (no *Node) monitorBlockHeaders() {
	for newHeader := range no.blockHeadersCh {
		err := no.blockHeadersDb.WriteHeaders(newHeader)
		if err != nil {
			logrus.Error(err)
		}
	}
}
