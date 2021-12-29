package protocol

import (
	"io"

	"github.com/sirupsen/logrus"
	"github.com/vulpemventures/go-elements/block"
	"github.com/vulpemventures/go-elements/transaction"
	"github.com/vulpemventures/neutrino-elements/pkg/binary"
)

// MsgBlock represents 'block' message.
type MsgBlock struct {
	block.Block
}

// UnmarshalBinary implements binary.Unmarshaler
func (blck *MsgBlock) UnmarshalBinary(r io.Reader) error {
	d := binary.NewDecoder(r)
	bytes, err := d.ReadUntilEOF()
	if err != nil {
		return err
	}

	logrus.Debugf("number of bytes: %+v", bytes.Len())

	decodedHeader, err := block.DeserializeHeader(&bytes)
	if err != nil {
		return err
	}

	blck.Header = decodedHeader
	blck.TransactionsData = &block.Transactions{
		Transactions: []*transaction.Transaction{},
	}
	return nil
}
