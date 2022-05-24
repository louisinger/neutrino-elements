package node

import (
	"io"

	"github.com/sirupsen/logrus"
	"github.com/vulpemventures/neutrino-elements/pkg/binary"
	"github.com/vulpemventures/neutrino-elements/pkg/peer"
	"github.com/vulpemventures/neutrino-elements/pkg/protocol"
)

const (
	// maxHeadersPerMsg is the maximum number of block headers that can be in
	// a single headers message.
	maxHeadersPerMsg = 2000
)

func (n node) handleHeaders(msgHeader *protocol.MessageHeader, p peer.Peer) (err error) {
	var headers protocol.MsgHeaders

	conn := p.Connection()
	lr := io.LimitReader(conn, int64(msgHeader.Length))

	if err := binary.NewDecoder(lr).Decode(&headers); err != nil {
		return err
	}

	startHeight := headers.Headers[0].Height
	stop := headers.Headers[len(headers.Headers)-1]

	logrus.Debugf("received %d headers from peer %s", len(headers.Headers), p.ID())
	for _, header := range headers.Headers {
		n.blockHeadersCh <- *header

		if header.Height > stop.Height {
			stop = header
		}

		if header.Height < startHeight {
			startHeight = header.Height
		}
	}
	logrus.Debugf("start height: %d, stop height: %d", startHeight, stop.Height)

	lastBlockHash, err := stop.Hash()
	if err != nil {
		return err
	}

	getcFilter := &protocol.MsgGetCFilters{
		FilterType:  0,
		StartHeight: startHeight,
		StopHash:    lastBlockHash,
	}

	msg, err := protocol.NewMessage("getcfilters", n.Network, getcFilter)
	if err != nil {
		logrus.Errorf("failed to create getcfilters message: %v", err)
		goto CheckResendMsg
	}

	err = n.sendMessage(conn, msg)
	if err != nil {
		logrus.Errorf("failed to send getcfilters to peer %s: %v", p.ID(), err)
		goto CheckResendMsg
	}

CheckResendMsg:
	// resend a getheaders message to the peer if needed
	if len(headers.Headers) > 0 && len(headers.Headers) >= maxHeadersPerMsg {
		locator, err := n.getLatestBlockLocator()
		if err != nil {
			return err
		}

		msg, err := protocol.NewMsgGetHeaders(n.Network, lastBlockHash, locator)
		if err != nil {
			return err
		}

		logrus.Debugf("sending getheaders to peer %s", p.ID())
		return n.sendMessage(conn, msg)
	}

	n.checkSync(nil) // check if the db is fully sync
	return err
}
