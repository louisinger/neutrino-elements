package node

import (
	"io"

	"github.com/vulpemventures/neutrino-elements/pkg/binary"
	"github.com/vulpemventures/neutrino-elements/pkg/peer"
	"github.com/vulpemventures/neutrino-elements/pkg/protocol"
)

const (
	// maxHeadersPerMsg is the maximum number of block headers that can be in
	// a single headers message.
	maxHeadersPerMsg = 3000
)

func (n node) handleHeaders(msgHeader *protocol.MessageHeader, p peer.Peer) error {
	var headers protocol.MsgHeaders

	conn := p.Connection()
	lr := io.LimitReader(conn, int64(msgHeader.Length))

	if err := binary.NewDecoder(lr).Decode(&headers); err != nil {
		return err
	}

	for _, header := range headers.Headers {
		n.blockHeadersCh <- *header
	}

	// resend a getheaders message to the peer if needed
	if len(headers.Headers) > 0 && len(headers.Headers) >= maxHeadersPerMsg {
		locator, err := n.getLatestBlockLocator()
		if err != nil {
			return err
		}

		last, err := headers.Headers[len(headers.Headers)-1].Hash()
		if err != nil {
			return err
		}

		msg, err := protocol.NewMsgGetHeaders(n.Network, last, locator)
		if err != nil {
			return err
		}

		n.sendMessage(conn, msg)
	} else {
		n.checkSync(nil) // check if the db is fully sync

	}
	return nil
}
