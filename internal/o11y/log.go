package o11y

import (
	"fmt"
	"io"
	stdlog "log"
	"net"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/go-logr/zerologr"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/diode"
	"github.com/rs/zerolog/log"
)

const (
	logMessageBufferSize = 1000
)

// init sets some zerolog global defaults we want to keep throughout the project.
func init() {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.TimestampFieldName = "timestamp"

	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		return path.Base(file) + ":" + strconv.Itoa(line)
	}

	zerolog.ErrorFieldName = "error.message"
	zerolog.ErrorStackFieldName = "error.stack"

	// remove v-level from zerologr wrapper.
	// The v-level is redundant with `level` emitted by zerolog.
	zerologr.VerbosityFieldName = ""
}

// SetGlobalLogger sets the log output in the stdlib log package and the
// zerolog global loggers.
func SetGlobalLogger(logger *zerolog.Logger) {
	// Rewire stdlib "log" global logger to our logger for dependencies
	// logging to `log.Default()...`
	stdlog.SetFlags(0)
	stdlog.SetOutput(logger)

	// Update zerolog global logger for packages/dependencies using this logger
	log.Logger = *logger

	// Set global logger in case context.Context is missing a contextual logger
	zerolog.DefaultContextLogger = logger
}

// NewLogger create a new logger writing to out.
// The logger will emit a timestamp, the caller's filename, and optionally
// emit the stacktrace for errors that carry a stack trace.
//
// The Debug and Trace level are samples.
// We allow up to 100 trace logs per minutes. Additional trace logs will be filtered out.
// Debug logs are sampled. Every 5th log will be filtered out once the limit of 1000 debug logs
// per minute is reached.
func NewLogger(out io.Writer, config *Config) zerolog.Logger {
	logger := zerolog.New(out).
		Sample(zerolog.LevelSampler{
			TraceSampler: &zerolog.BurstSampler{
				Burst:  100,
				Period: 1 * time.Minute,
			},
			DebugSampler: &zerolog.BurstSampler{
				Burst:       1000,
				Period:      1 * time.Minute,
				NextSampler: &zerolog.BasicSampler{N: 5},
			},
		}).
		With().
		Timestamp().
		Caller().
		Stack().
		Logger().
		Level(config.LogLevel)

	return logger
}

// NewLogOutput creates a new io.Writer for use with a new logger.
//
// The log writer will write to stderr by default.
// If stderr is a tty we will produce colored human readable log lines.
// Otherwise JSON documents will be written.
func NewLogOutput(consoleJSON bool) io.WriteCloser {
	var out io.Writer
	if isatty.IsTerminal(os.Stderr.Fd()) && !consoleJSON {
		// produce pretty logs when logging to terminal
		writer := zerolog.NewConsoleWriter()
		writer.Out = os.Stderr
		writer.TimeFormat = time.RFC3339Nano
		out = writer
	} else {
		out = os.Stderr
	}

	// Use diode with waiter. This has some overhead on the log producers as we need
	// to wake up the waiter by using a cond.
	return diode.NewWriter(out, logMessageBufferSize, 0, func(missed int) {
		fmt.Printf("Dropped %v log messages", missed) //nolint:forbidigo // logger is allowed to print to stdout
	})
}

// NewTCPLogOutput creates a new TCP log output.
func NewTCPLogOutput(addr string) (io.WriteCloser, error) {
	client, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	return diode.NewWriter(client, logMessageBufferSize, 0, func(missed int) {
		fmt.Printf("Dropped %v log messages", missed) //nolint:forbidigo // logger is allowed to print to stdout
	}), nil
}
