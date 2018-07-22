package adapter

import (
	"fmt"
	"github.com/orbs-network/orbs-spec/types/go/primitives"
	"github.com/orbs-network/orbs-spec/types/go/protocol/gossipmessages"
)

type TransportData struct {
	SenderPublicKey     primitives.Ed25519Pkey
	RecipientMode       gossipmessages.RecipientsListMode
	RecipientPublicKeys []primitives.Ed25519Pkey
	Payloads            [][]byte // the first payload is normally gossipmessages.Header
}

type Transport interface {
	RegisterListener(listener TransportListener, myNodeId string)
	Send(header *gossipmessages.Header, payloads [][]byte) error
	Join()
}

type TransportListener interface {
	OnTransportMessageReceived(payloads [][]byte)
}

type ErrCorruptData struct {
}

func (e *ErrCorruptData) Error() string {
	return fmt.Sprintf("transport data is corrupt and missing required fields")
}

type ErrTransportFailed struct {
	Data *TransportData
}

func (e *ErrTransportFailed) Error() string {
	return fmt.Sprintf("transport failed to send: %v", e.Data)
}
