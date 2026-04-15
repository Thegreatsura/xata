package o11y

import (
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"xata/internal/envcfg"
)

// Config for tracing
type Config struct {
	// Is tracing enabled?
	Tracing bool `env:"XATA_TRACING" env-default:"false" env-description:"enable/disable tracing and metrics"`

	ProfilingServer string `env:"XATA_PROFILING_SERVER" env-default:"" env-description:"configure optional profiling http server"`

	Profiling ContinuousProfilingMode `env:"XATA_CONT_PROFILING" env-default:"none" env-description:"configure continuous profiling"`

	ProfileTypes profileTypes `env:"XATA_CONT_PROFILING_TYPES" env-default:"cpu,memory" env-description:"configure the profiling types"`

	IDStyle idstyleConfig `env:"XATA_OTEL_ID_STYLE" env-default:"datadog" env-description:"configure trace ID formatting for correlating logs with traces"`

	MetricsPeriod time.Duration `env:"XATA_OTEL_METRICS_PERIOD" env-default:"60s" env-description:"metrics collection period"`

	ConsoleJSON bool `env:"XATA_LOG_JSON" env-default:"false" env-description:"enable/disable json log output when logging to the console"`

	LogLevel zerolog.Level `env:"XATA_LOG_LEVEL" env-default:"trace" env-description:"configure minimum application log level, valid values are trace, debug, info, warn, error, fatal, panic"`

	LogTCPOut string `env:"XATA_LOG_OUT_TCP" env-default:"" env-description:"configure and enable tcp log output"`
}

// GetConfigFromEnv returns tracing config
func GetConfigFromEnv() (config Config, err error) {
	err = envcfg.Read(&config)
	return
}

type idstyleConfig struct {
	style TraceIDStyle
}

func (cfg *idstyleConfig) SetValue(s string) error {
	if s == "" {
		cfg.style = PlainIDStyle
		return nil
	}

	switch s {
	case "datadog":
		cfg.style = DatadogIDStyle
	case "plain", "hex":
		cfg.style = PlainIDStyle
	default:
		return fmt.Errorf("unknown id style: %s", s)
	}
	return nil
}

type ContinuousProfilingMode struct {
	mode profilingSupport
}

func NoProfiling() ContinuousProfilingMode {
	return ContinuousProfilingMode{(*noContinuousProfiling)(nil)}
}

func (cfg *ContinuousProfilingMode) GetValue() profilingSupport {
	if cfg.mode == nil {
		return (*noContinuousProfiling)(nil)
	}
	return cfg.mode
}

func (cfg *ContinuousProfilingMode) SetValue(s string) error {
	switch s {
	case "", "none":
		*cfg = NoProfiling()
	case "pyroscope":
		cfg.mode = &pyroscopeProfiling{}
	default:
		return fmt.Errorf("%v not supported", s)
	}
	return nil
}

type profileTypes struct {
	list []profileType
}

func (p *profileTypes) SetValue(s string) error {
	names := strings.SplitSeq(s, ",")
	for name := range names {
		pt, err := parseProfileType(name)
		if err != nil {
			return err
		}
		p.list = append(p.list, pt)
	}
	return nil
}

type profileType uint8

const (
	profileTypeCPU profileType = iota + 1
	profileTypeMemory
	profileTypeBlock
	profileTypeMutex
	profileTypeGoroutine
)

func parseProfileType(s string) (profileType, error) {
	switch strings.ToLower(s) {
	case "cpu":
		return profileTypeCPU, nil
	case "memory":
		return profileTypeMemory, nil
	case "block":
		return profileTypeBlock, nil
	case "mutex":
		return profileTypeMutex, nil
	case "goroutine":
		return profileTypeGoroutine, nil
	default:
		return 0, fmt.Errorf("unknown profile type %v", s)
	}
}

func (p *profileType) Set(s string) (err error) {
	*p, err = parseProfileType(s)
	return err
}
