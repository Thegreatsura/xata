package bench

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"reflect"

	"github.com/jackc/pgx/v5/pgproto3"
)

// BackendEndpoint is an endpoint that receives backend messages and sends frontend messages.
// It represents the client-side of a PostgreSQL connection.
type BackendEndpoint = Endpoint[pgproto3.BackendMessage, pgproto3.FrontendMessage]

// FrontendEndpoint is an endpoint that receives frontend messages and sends backend messages.
// It represents the server-side of a PostgreSQL connection.
type FrontendEndpoint = Endpoint[pgproto3.FrontendMessage, pgproto3.BackendMessage]

// Endpoint is a generic interface that can receive requests and send responses.
type Endpoint[Req, Resp any] interface {
	Receiver[Req]
	Sender[Resp]
}

// Receiver is a generic interface for an entity that can receive a message of type T.
type Receiver[T any] interface {
	Receive() (T, error)
}

// Sender is a generic interface for an entity that can send a message of type T.
type Sender[T any] interface {
	Send(T)
	Flush() error
}

// MessageReader reads PostgreSQL protocol messages from a pgproto3 JSON stream.
type MessageReader struct {
	err error
	dec *json.Decoder
}

// NewMessageReader creates a new MessageReader that reads from the provided io.Reader.
func NewMessageReader(r io.Reader) *MessageReader {
	return &MessageReader{
		dec: json.NewDecoder(r),
	}
}

// Err returns the last error that occurred during reading, except for io.EOF.
// Use Err after processing all messages to check if there was an error.
func (r *MessageReader) Err() error {
	if r.err != nil && !errors.Is(r.err, io.EOF) {
		return r.err
	}
	return nil
}

// Each iterates over the messages from the reader, calling the yield function for each message.
//
// Each implements the iter.Seq[any] interface.
func (r *MessageReader) Each(yield func(any) bool) {
	for msg := r.Next(); msg != nil && r.err == nil; msg = r.Next() {
		if !yield(msg) {
			break
		}
	}
}

// Next reads the next message from the stream.
// It returns nil if there are no more messages or an error occurred.
// Use Err() to check for errors.
func (r *MessageReader) Next() any {
	msg, err := r.next()
	if err != nil {
		r.err = err
		return nil
	}
	return msg
}

func (r *MessageReader) next() (any, error) {
	if r.err != nil {
		return nil, r.err
	}

	var rawMsg json.RawMessage
	if err := r.dec.Decode(&rawMsg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to decode message: %w", err)
	}

	var msgType struct {
		Type string `json:"Type"`
	}
	if err := json.Unmarshal(rawMsg, &msgType); err != nil {
		return nil, fmt.Errorf("failed to decode message type: %w", err)
	}

	descr, ok := pgMessageTable[msgType.Type]
	if !ok {
		return nil, fmt.Errorf("unknown message type: %s", msgType.Type)
	}

	msg := descr.Make()
	if err := json.Unmarshal(rawMsg, msg); err != nil {
		return nil, fmt.Errorf("failed to decode message: %w", err)
	}

	return msg, nil
}

// LoadMessages reads all PostgreSQL protocol messages from a JSON stream and returns them as a slice.
func LoadMessages(r io.Reader) ([]any, error) {
	reader := NewMessageReader(r)
	msgs := make([]any, 0, 100)
	for msg := range reader.Each {
		msgs = append(msgs, msg)
	}
	if err := reader.Err(); err != nil {
		return nil, err
	}
	return msgs, nil
}

// pgMessageTable registers all PostgreSQL protocol messages with their types,
// so we can construct and decode the correct message types from a JSON stream.
//
// We verify the types are indeed postgres message types during the construction of the table.
var pgMessageTable = buildPGMessageTable(
	pgMessage[pgproto3.AuthenticationCleartextPassword](),
	pgMessage[pgproto3.AuthenticationGSSContinue](),
	pgMessage[pgproto3.AuthenticationGSS](),
	pgMessage[pgproto3.AuthenticationMD5Password](),
	pgMessage[pgproto3.AuthenticationOk](),
	pgMessage[pgproto3.AuthenticationSASLContinue](),
	pgMessage[pgproto3.AuthenticationSASLFinal](),
	pgMessage[pgproto3.AuthenticationSASL](),
	pgMessage[pgproto3.BackendKeyData](),
	pgMessage[pgproto3.BindComplete](),
	pgMessage[pgproto3.Bind](),
	pgMessage[pgproto3.CancelRequest](),
	pgMessage[pgproto3.CloseComplete](),
	pgMessage[pgproto3.Close](),
	pgMessage[pgproto3.CommandComplete](),
	pgMessage[pgproto3.CopyBothResponse](),
	pgMessage[pgproto3.CopyData](),
	pgMessage[pgproto3.CopyDone](),
	pgMessage[pgproto3.CopyFail](),
	pgMessage[pgproto3.CopyInResponse](),
	pgMessage[pgproto3.CopyOutResponse](),
	pgMessage[pgproto3.DataRow](),
	pgMessage[pgproto3.Describe](),
	pgMessage[pgproto3.EmptyQueryResponse](),
	pgMessage[pgproto3.ErrorResponse](),
	pgMessage[pgproto3.Execute](),
	pgMessage[pgproto3.Flush](),
	pgMessage[pgproto3.FunctionCall](),
	pgMessage[pgproto3.FunctionCallResponse](),
	pgMessage[pgproto3.GSSEncRequest](),
	pgMessage[pgproto3.GSSResponse](),
	pgMessage[pgproto3.NoData](),
	pgMessage[pgproto3.NoticeResponse](),
	pgMessage[pgproto3.NotificationResponse](),
	pgMessage[pgproto3.ParameterDescription](),
	pgMessage[pgproto3.ParameterStatus](),
	pgMessage[pgproto3.Parse](),
	pgMessage[pgproto3.ParseComplete](),
	pgMessage[pgproto3.PasswordMessage](),
	pgMessage[pgproto3.PortalSuspended](),
	pgMessage[pgproto3.Query](),
	pgMessage[pgproto3.ReadyForQuery](),
	pgMessage[pgproto3.RowDescription](),
	pgMessage[pgproto3.SASLInitialResponse](),
	pgMessage[pgproto3.SASLResponse](),
	pgMessage[pgproto3.SSLRequest](),
	pgMessage[pgproto3.StartupMessage](),
	pgMessage[pgproto3.Sync](),
	pgMessage[pgproto3.Terminate](),
)

func buildPGMessageTable(descr ...pgMessageDescr) map[string]pgMessageDescr {
	table := make(map[string]pgMessageDescr, len(descr))
	for _, descr := range descr {
		table[descr.Name] = descr
	}
	return table
}

type pgMessageDescr struct {
	Name string
	Make func() any
}

func pgMessage[T any]() pgMessageDescr {
	var tmp T
	name := reflect.TypeOf(tmp).Name()

	// Double check the type given really is a PostgreSQL protocol message.
	if _, isBackend := any(&tmp).(pgproto3.BackendMessage); !isBackend {
		if _, isFrontend := any(&tmp).(pgproto3.FrontendMessage); !isFrontend {
			panic(fmt.Sprintf("%T does not implement BackendMessage or FrontendMessage", tmp))
		}
	}

	return pgMessageDescr{
		Name: name,
		Make: func() any {
			return new(T)
		},
	}
}

// IgnoreStartupSequence is an iterator adapter that filters out the initial PostgreSQL startup sequence.
// It skips messages from StartupMessage to the first ReadyForQuery.
// If the first message is not a StartupMessage, it doesn't skip anything.
func IgnoreStartupSequence(it iter.Seq[any]) iter.Seq[any] {
	return func(yield func(any) bool) {
		// if first message is not StartupMessage we are done. Otherwise iterate until ReadyForQuery.
	startupLoop:
		for msg := range it {
			if _, ok := msg.(pgproto3.StartupMessage); !ok {
				yield(msg)
				break
			}
			for msg := range it {
				if _, ok := msg.(pgproto3.ReadyForQuery); ok {
					break startupLoop
				}
			}
		}

		for msg := range it {
			yield(msg)
		}
	}
}
