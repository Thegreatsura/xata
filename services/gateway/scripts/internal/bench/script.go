package bench

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/elastic/go-concert/ctxtool"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/rs/zerolog/log"
)

// BackendScript is a script for a backend endpoint, which receives frontend messages
// and sends backend messages.
// It represents the server-side of a PostgreSQL connection.
type BackendScript = Script[pgproto3.FrontendMessage, pgproto3.BackendMessage]

// FrontendScript is a script for a frontend endpoint, which receives backend messages
// and sends frontend messages.
// It represents the client-side of a PostgreSQL connection.
type FrontendScript = Script[pgproto3.BackendMessage, pgproto3.FrontendMessage]

// Script is a sequence of steps to be executed against an endpoint.
type Script[Req, Resp any] []Step[Req, Resp]

// BackendLoadConfig is the configuration for loading a backend script.
type BackendLoadConfig = LoadConfig[pgproto3.FrontendMessage, pgproto3.BackendMessage]

// FrontendLoadConfig is the configuration for loading a frontend script.
type FrontendLoadConfig = LoadConfig[pgproto3.BackendMessage, pgproto3.FrontendMessage]

// LoadConfig provides functions to compile raw pgwire messages into script steps.
type LoadConfig[Req, Resp any] struct {
	CompileFrontendStep func(pgproto3.FrontendMessage) (Step[Req, Resp], error)
	CompileBackendStep  func(pgproto3.BackendMessage) (Step[Req, Resp], error)
}

// LoadScript reads a script from a file, parses it, and returns a runnable Script.
// The script file is a sequence of pgwire messages.
//
// It can be compressed with gzip if it has a .gz or .gzip extension.
func LoadScript[Req, Resp any](ctx context.Context, path string, name string, config LoadConfig[Req, Resp]) (Script[Req, Resp], error) {
	scriptFileName := filepath.Join(path, name)
	log.Ctx(ctx).Debug().Str("script", scriptFileName).Msg("Loading script")
	defer log.Ctx(ctx).Debug().Msg("Script loaded")

	scriptFile, err := os.Open(scriptFileName)
	if err != nil {
		return nil, fmt.Errorf("failed to open script file: %w", err)
	}
	_, cancel := ctxtool.WithFunc(ctx, func() {
		scriptFile.Close()
	})
	defer cancel()

	// Create a reader that may decompress the input based on file extension
	var reader io.Reader = scriptFile
	if isCompressedFile(scriptFileName) {
		reader, err = createDecompressedReader(scriptFile, scriptFileName)
		if err != nil {
			return nil, fmt.Errorf("failed to create decompressed reader: %w", err)
		}
	}

	var script Script[Req, Resp]
	msgReader := NewMessageReader(reader)
	for msg := range IgnoreStartupSequence(msgReader.Each) {
		if msg == nil {
			return nil, errors.New("unexpected EOF")
		}

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		log.Ctx(ctx).Debug().Interface("msg", msg).Msg("Processing message")

		var step Step[Req, Resp]
		switch msg := msg.(type) {
		case pgproto3.FrontendMessage:
			step, err = config.CompileFrontendStep(msg)
		case pgproto3.BackendMessage:
			step, err = config.CompileBackendStep(msg)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to create step: %w", err)
		}

		// Merge consecutive awaitMessageStep steps
		isMerged := false
		if len(script) > 0 {
			switch current := step.(type) {
			case *awaitMessageStep[Req, Resp]:
				if last, ok := script[len(script)-1].(*awaitMessageStep[Req, Resp]); ok {
					last.count += current.count
					isMerged = true
				}
			case *sendMessagesStep[Req, Resp]:
				if last, ok := script[len(script)-1].(*sendMessagesStep[Req, Resp]); ok {
					last.msgs = append(last.msgs, current.msgs...)
					isMerged = true
				}
			}
		}

		if !isMerged {
			script = append(script, step)
		}
	}
	if err := msgReader.Err(); err != nil {
		return nil, fmt.Errorf("failed to read script file: %w", err)
	}
	if len(script) == 0 {
		return nil, errors.New("empty script")
	}

	script[0] = awaitOptionalClose(script[0])
	return script, nil
}

// isCompressedFile determines if a file is compressed based on its extension
func isCompressedFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".gz", ".gzip":
		return true
	default:
		return false
	}
}

// createDecompressedReader creates an appropriate decompressed reader based on file extension
func createDecompressedReader(file io.Reader, filename string) (io.Reader, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".gz", ".gzip":
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		return gzReader, nil
	default:
		return nil, fmt.Errorf("unsupported compression format: %s", ext)
	}
}

// Run executes all the steps in the script sequentially against the given endpoint.
func (s Script[Req, Resp]) Run(ctx context.Context, backend Endpoint[Req, Resp]) error {
	for i, step := range s {
		done, err := step.Exec(ctx, backend)
		if err != nil {
			return fmt.Errorf("failed to execute step %d: %w", i, err)
		}
		if done {
			break
		}
	}
	return nil
}

// Step is a single action in a script, like sending or receiving a message.
type Step[Req, Resp any] interface {
	// Exec executes the step against the given endpoint.
	// It returns true if the script should stop executing.
	Exec(ctx context.Context, backend Endpoint[Req, Resp]) (done bool, err error)
}

// BackendStep is a step in a backend script.
type BackendStep = Step[pgproto3.FrontendMessage, pgproto3.BackendMessage]

// FrontendStep is a step in a frontend script.
type FrontendStep = Step[pgproto3.BackendMessage, pgproto3.FrontendMessage]

// StepFunc is an adapter to allow the use of ordinary functions as script steps.
type StepFunc[Req, Resp any] func(ctx context.Context, backend Endpoint[Req, Resp]) (done bool, err error)

// Exec executes the wrapped function.
func (f StepFunc[Req, Resp]) Exec(ctx context.Context, backend Endpoint[Req, Resp]) (done bool, err error) {
	return f(ctx, backend)
}

func awaitOptionalClose[Req, Resp any](inner Step[Req, Resp]) Step[Req, Resp] {
	return StepFunc[Req, Resp](func(ctx context.Context, backend Endpoint[Req, Resp]) (done bool, err error) {
		done, err = inner.Exec(ctx, backend)
		if err != nil {
			return done, err
		}
		if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
			return true, nil
		}
		return false, err
	})
}

type awaitMessageStep[Req, Resp any] struct {
	count int
}

func (s *awaitMessageStep[Req, Resp]) Exec(ctx context.Context, backend Endpoint[Req, Resp]) (done bool, err error) {
	for i := 0; i < s.count; i++ {
		_, err := backend.Receive()
		if err != nil {
			return true, fmt.Errorf("failed to receive message: %w", err)
		}
	}
	return false, nil
}

// AwaitMessages creates a step that waits to receive a specific number of messages.
func AwaitMessages[Req, Resp any](count int) Step[Req, Resp] {
	type stepType = awaitMessageStep[Req, Resp]
	return &stepType{count: count}
}

// AwaitValidateMessage creates a step that waits for a message and validates
// that it's equal to the expected message.
func AwaitValidateMessage[Req, Resp any](msg Req) Step[Req, Resp] {
	return StepFunc[Req, Resp](func(ctx context.Context, endpoint Endpoint[Req, Resp]) (done bool, err error) {
		received, err := endpoint.Receive()
		if err != nil {
			return true, fmt.Errorf("failed to receive message: %w", err)
		}
		if !reflect.DeepEqual(msg, received) {
			return true, fmt.Errorf("expected message %v, got %v", msg, received)
		}
		return false, nil
	})
}

type sendMessagesStep[Req, Resp any] struct {
	msgs []Resp
}

func (s *sendMessagesStep[Req, Resp]) Exec(ctx context.Context, backend Endpoint[Req, Resp]) (done bool, err error) {
	for _, msg := range s.msgs {
		backend.Send(msg)
	}
	err = backend.Flush()
	return err != nil, err
}

// SendMessages creates a step that sends one or more messages.
func SendMessages[Req, Resp any](msgs ...Resp) Step[Req, Resp] {
	return &sendMessagesStep[Req, Resp]{msgs: msgs}
}

// EncodedMessage holds a pre-encoded message to avoid re-encoding it every time
// it's sent. It implements both pgproto3.BackendMessage and pgproto3.FrontendMessage,
// but Decode will always return an error.
type EncodedMessage[T any] struct {
	original []T
	encoded  []byte
}

// NewEncodedMessage creates a new EncodedMessage by encoding the provided message.
func NewEncodedMessage[T interface {
	Encode(buf []byte) ([]byte, error)
}](msg T) (*EncodedMessage[T], error) {
	encoded, err := msg.Encode(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to encode message: %w", err)
	}
	return &EncodedMessage[T]{original: []T{msg}, encoded: encoded}, nil
}

// Backend implements pgproto3.BackendMessage.
func (m *EncodedMessage[T]) Backend() {}

// Frontend implements pgproto3.FrontendMessage.
func (m *EncodedMessage[T]) Frontend() {}

// Decode is a no-op that returns an error, as these messages are not meant to be decoded.
func (m *EncodedMessage[T]) Decode(data []byte) error {
	return errors.New("must not decode into script messages")
}

// Encode appends the pre-encoded message data to the buffer.
func (m *EncodedMessage[T]) Encode(buf []byte) ([]byte, error) {
	return append(buf, m.encoded...), nil
}
