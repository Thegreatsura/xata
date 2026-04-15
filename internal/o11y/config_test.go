package o11y

import (
	"testing"

	"xata/internal/envcfg"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestLogLevelMapping(t *testing.T) {
	t.Run("when env is missing, log level should be set to trace", func(t *testing.T) {
		conf := Config{}
		err := envcfg.Read(&conf)
		assert.True(t, err == nil)

		assert.Equal(t, zerolog.TraceLevel, conf.LogLevel)
	})

	t.Run("when env is set, log level should be parsed and set correctly", func(t *testing.T) {
		t.Setenv("XATA_LOG_LEVEL", "error")

		conf := Config{}
		err := envcfg.Read(&conf)
		assert.True(t, err == nil)

		assert.Equal(t, zerolog.ErrorLevel, conf.LogLevel)
	})

	t.Run("when env is set incorrectly, error should be reported", func(t *testing.T) {
		t.Setenv("XATA_LOG_LEVEL", "something")

		conf := Config{}
		err := envcfg.Read(&conf)
		assert.True(t, err != nil)
	})
}
