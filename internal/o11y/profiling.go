package o11y

import (
	"github.com/grafana/pyroscope-go"
)

type profilingSupport interface {
	Start(*Config) (profiler, error)
}

type profiler interface {
	Stop() error
}

type noContinuousProfiling struct{}

func (*noContinuousProfiling) Start(_ *Config) (profiler, error) {
	return (*noopProfiler)(nil), nil
}

type noopProfiler struct{}

func (*noopProfiler) Stop() error { return nil }

type pyroscopeProfiling struct{}

func (pyroscopeProfiling) Start(cfg *Config) (profiler, error) {
	var profileTypes []pyroscope.ProfileType
	for _, pt := range cfg.ProfileTypes.list {
		switch pt {
		case profileTypeCPU:
			profileTypes = append(profileTypes, pyroscope.ProfileCPU)
		case profileTypeMemory:
			profileTypes = append(profileTypes,
				pyroscope.ProfileAllocObjects,
				pyroscope.ProfileAllocSpace,
				pyroscope.ProfileInuseObjects,
				pyroscope.ProfileInuseSpace)
		}
	}
	if len(profileTypes) == 0 {
		return (*noopProfiler)(nil), nil
	}

	return pyroscope.Start(pyroscope.Config{
		ApplicationName: "xerver",
		ServerAddress:   "http://localhost:4040",
		Logger:          nil,
		ProfileTypes:    profileTypes,
	})
}
