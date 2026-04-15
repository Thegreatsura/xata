package o11y

import (
	"context"

	"xata/internal/envcfg"

	"github.com/rs/zerolog"
)

type testConfig struct {
	LogLevel zerolog.Level `env:"XATA_TEST_LOG_LEVEL" env-default:"trace" env-description:"configure minimum application log level, valid values are trace, debug, info, warn, error, fatal, panic"`
}

type testingLog interface {
	zerolog.TestingLog

	Cleanup(fn func())
}

func NewTestSystem(t testingLog) *System {
	return &System{
		logger: NewTestLogger(t),
	}
}

func NewTestService(t testingLog) O {
	return NewTestSystem(t).ForService(context.Background(), "", "")
}

// NewTestLogger creates a new logger emitting all logs to a testing.TB instance.
func NewTestLogger(t zerolog.TestingLog) zerolog.Logger {
	var conf testConfig
	if err := envcfg.Read(&conf); err != nil {
		panic(err)
	}

	return NewLogger(zerolog.NewTestWriter(t), &Config{LogLevel: conf.LogLevel})
}

// NewTestServiceContext creates a new context with a new logger writing to t.
func NewTestServiceContext(t testingLog, ctx context.Context) context.Context {
	o := NewTestService(t)
	return o.WithContext(ctx)
}
