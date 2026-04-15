package initiator

var (
	ErrorSSLRequired      = ErrSSLRequired{}
	ErrorStartupMsgCode   = ErrStartupMsgCode{}
	ErrorStartupMsgLength = ErrStartupMsgLength{}
)

type ErrSSLRequired struct{}

func (e ErrSSLRequired) Error() string {
	return "SSL required"
}

// ErrStartupMsgCode is returned when the startup message code is not recognized
type ErrStartupMsgCode struct{}

func (e ErrStartupMsgCode) Error() string {
	return "unknown startup message code"
}

// ErrStartupMsgLength is returned when the length of the startup message is invalid. Usually due to clients trying to initialize non-WIRE connections
type ErrStartupMsgLength struct{}

func (e ErrStartupMsgLength) Error() string {
	return "invalid length of startup packet"
}
