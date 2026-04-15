package errors

import (
	"net/http"
	"strings"
)

type IdentifierError struct {
	Key, Value string
	Reason     error
}

func (e IdentifierError) Error() string {
	var buf strings.Builder

	if e.Key != "" {
		buf.WriteRune('{')
		buf.WriteString(e.Key)
		buf.WriteString("} ")
	}
	buf.WriteString("invalid identifier")

	if e.Value != "" {
		buf.WriteString(" [")
		buf.WriteString(e.Value)
		buf.WriteRune(']')
	}

	if e.Reason != nil {
		buf.WriteString(", ")
		buf.WriteString(e.Reason.Error())
	}

	return buf.String()
}

func (e IdentifierError) Unwrap() error {
	return e.Reason
}

func (e IdentifierError) StatusCode() int {
	return http.StatusBadRequest
}
