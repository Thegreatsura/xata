package initiator

import (
	"errors"
)

var errExpectedEmptyMessage = errors.New("expected empty message")

// SSLYesResponse is a postgres message type to indicate SSL is supported. Implements pgproto3.BackendMessage.
type SSLYesResponse struct{}

func (*SSLYesResponse) Backend() {}

func (*SSLYesResponse) Decode(data []byte) error {
	if len(data) != 0 {
		return errExpectedEmptyMessage
	}
	return nil
}

func (SSLYesResponse) Encode(dst []byte) ([]byte, error) {
	return append(dst, 'S'), nil
}

type NoResponse struct{}

func (*NoResponse) Backend() {}

func (*NoResponse) Decode(data []byte) error {
	if len(data) != 0 {
		return errExpectedEmptyMessage
	}
	return nil
}

func (NoResponse) Encode(dst []byte) ([]byte, error) {
	return append(dst, 'N'), nil
}
