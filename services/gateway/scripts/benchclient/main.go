package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	"xata/services/gateway/scripts/internal/bench"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var connString string
	var scriptFolder string
	var debug bool
	var validate bool
	var numConnections int
	var benchTime time.Duration
	var benchCount int
	var loopCount int
	var outfile string
	flag.StringVar(&connString, "pg", "postgres://postgres@localhost:5432/postgres", "connection string")
	flag.StringVar(&scriptFolder, "s", ".", "script folder")
	flag.BoolVar(&debug, "d", false, "debug mode")
	flag.BoolVar(&validate, "validate", false, "validate received messages")
	flag.IntVar(&numConnections, "n", 1, "number of connections")
	flag.DurationVar(&benchTime, "time", 0, "benchmark time")
	flag.IntVar(&benchCount, "c", 0, "benchmark count")
	flag.IntVar(&loopCount, "l", 1, "loop count per benchmark run")
	flag.StringVar(&outfile, "o", "", "output file")
	flag.Parse()

	log.Printf("Connection string: %s\n", connString)
	log.Printf("Script folder: %s\n", scriptFolder)
	log.Printf("Debug mode: %t\n", debug)
	log.Printf("Validate messages: %t\n", validate)
	log.Printf("Number of connections: %d\n", numConnections)
	log.Printf("Benchmark time: %s\n", benchTime)
	log.Printf("Benchmark count: %d\n", benchCount)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if benchCount <= 0 && benchTime <= 0 {
		return fmt.Errorf("either bench count (-c) or bench time (-time) must be > 0")
	}

	logLevel := zerolog.ErrorLevel
	if debug {
		logLevel = zerolog.DebugLevel
	}
	logger := zerolog.New(os.Stderr).Level(logLevel).With().Timestamp().Logger()
	ctx = logger.WithContext(ctx)

	if flag.NArg() != 1 {
		flag.Usage()
		return flag.ErrHelp
	}
	scriptName := flag.Arg(0)

	compileBackendStep := createBackendStep
	if validate {
		compileBackendStep = createValidatingBackendStep
	}

	script, err := bench.LoadScript(ctx, scriptFolder, scriptName, bench.FrontendLoadConfig{
		CompileFrontendStep: createFrontendStep,
		CompileBackendStep:  compileBackendStep,
	})
	if err != nil {
		return fmt.Errorf("failed to load script: %w", err)
	}

	config, err := pgx.ParseConfig(connString)
	if err != nil {
		return fmt.Errorf("failed to parse connection string: %w", err)
	}
	config.Password = filepath.Base(scriptName)

	log.Ctx(ctx).Info().Msg("Running script")
	defer log.Ctx(ctx).Info().Msg("Script finished")

	benchmarkStats := BenchmarkRunSummary{
		Results: make([]BenchmarkResult, numConnections),
	}

	var wg sync.WaitGroup
	wg.Add(numConnections)

	benchmark := NewBenchmark(benchTime, benchCount)
	for i := 0; i < numConnections; i++ {
		go func(i int) {
			defer wg.Done()

			logger := log.Ctx(ctx).With().Int("connection", i).Logger()
			ctx := logger.WithContext(ctx)

			logger.Info().Str("connString", connString).Msg("Connecting to database")
			pgConn, err := pgx.ConnectConfig(ctx, config)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to connect to database")
				return
			}
			defer pgConn.Close(context.Background())

			frontend := pgConn.PgConn().Frontend()
			benchmarkStats.Results[i] = benchmark.Run(ctx, func(b B) {
				for b.Loop() {
					for range loopCount {
						if err := script.Run(ctx, frontend); err != nil {
							b.Fatal(err.Error())
						}
					}
				}
			})
		}(i)
	}
	wg.Wait()

	if len(benchmarkStats.Results) == 0 {
		return fmt.Errorf("no results")
	}

	readDuration := func(result BenchmarkResult) float64 {
		return float64(result.T)
	}
	readNsPerOp := func(result BenchmarkResult) float64 {
		return float64(result.NsPerOp())
	}

	benchmarkStats.Summary = SummaryResult{
		AvgDuration:    time.Duration(averageFunc(benchmarkStats.Results, readDuration)),
		MedianDuration: time.Duration(medianFunc(benchmarkStats.Results, readDuration)),
		AvgNsPerOp:     time.Duration(averageFunc(benchmarkStats.Results, readNsPerOp)),
		MedianNsPerOp:  time.Duration(medianFunc(benchmarkStats.Results, readNsPerOp)),
	}

	log.Ctx(ctx).Info().
		Str("avg_duration", benchmarkStats.Summary.AvgDuration.String()).
		Str("median_duration", benchmarkStats.Summary.MedianDuration.String()).
		Str("avg_duration_per_loop", benchmarkStats.Summary.AvgNsPerOp.String()).
		Str("median_duration_per_loop", benchmarkStats.Summary.MedianNsPerOp.String()).
		Msg("Benchmark stats")

	if outfile != "" {
		json, err := json.MarshalIndent(benchmarkStats, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal benchmark stats: %w", err)
		}
		if err := os.WriteFile(outfile, json, 0o600); err != nil {
			return fmt.Errorf("write benchmark stats to file: %w", err)
		}
	}

	return nil
}

func averageFunc[T any](results []T, f func(T) float64) float64 {
	var sum, compensation float64
	for _, result := range results {
		val := f(result)
		compensatedVal := val - compensation
		newSum := sum + compensatedVal
		compensation = (newSum - sum) - compensatedVal
		sum = newSum
	}
	return sum / float64(len(results))
}

func medianFunc[T any](results []T, f func(T) float64) float64 {
	values := make([]float64, len(results))
	for i, result := range results {
		values[i] = f(result)
	}
	sort.Float64s(values)
	return values[len(values)/2]
}

func createFrontendStep(msg pgproto3.FrontendMessage) (bench.FrontendStep, error) {
	emsg, err := bench.NewEncodedMessage(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to encode message: %w", err)
	}
	return bench.SendMessages[pgproto3.BackendMessage, pgproto3.FrontendMessage](emsg), nil
}

func createBackendStep(_ pgproto3.BackendMessage) (bench.FrontendStep, error) {
	return bench.AwaitMessages[pgproto3.BackendMessage, pgproto3.FrontendMessage](1), nil
}

func createValidatingBackendStep(msg pgproto3.BackendMessage) (bench.FrontendStep, error) {
	return bench.AwaitValidateMessage[pgproto3.BackendMessage, pgproto3.FrontendMessage](msg), nil
}
