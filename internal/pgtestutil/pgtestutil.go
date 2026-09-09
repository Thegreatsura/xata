// Package pgtestutil hands tests a Postgres database to work against without
// paying for a container start each time.
package pgtestutil

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const image = "postgres:16-alpine"

// startContainer runs at most once per test binary. The container is left to
// the testcontainers reaper rather than terminated by a t.Cleanup, so that it
// outlives whichever test happened to ask for the first database.
var startContainer = sync.OnceValues(func() (*postgres.PostgresContainer, error) {
	return postgres.Run(context.Background(), image,
		// One instance now backs every database in the package, where each used
		// to get a container to itself, so the default connection ceiling has
		// to cover all of them at once.
		testcontainers.WithCmdArgs("-c", "max_connections=200"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
})

var databases atomic.Uint64

// DSN returns a connection string for an empty database of its own. Callers in
// a package share one container, so each database costs a CREATE DATABASE
// rather than a container start.
func DSN(ctx context.Context, t *testing.T) string {
	t.Helper()

	dsn, err := createDatabase(ctx, fmt.Sprintf("test_%d", databases.Add(1)), "")
	require.NoError(t, err, "create database")

	return dsn
}

// createDatabase makes an empty database, or a copy of template when it is not
// empty, and returns a connection string pointing at it.
func createDatabase(ctx context.Context, name, template string) (string, error) {
	container, err := startContainer()
	if err != nil {
		return "", fmt.Errorf("start postgres container: %w", err)
	}

	admin := container.MustConnectionString(ctx, "sslmode=disable")
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		return "", fmt.Errorf("connect to postgres: %w", err)
	}
	defer conn.Close(ctx)

	stmt := "CREATE DATABASE " + pgx.Identifier{name}.Sanitize()
	if template != "" {
		stmt += " TEMPLATE " + pgx.Identifier{template}.Sanitize()
	}

	// Copying a database briefly conflicts with another copy of the same
	// template, so give way and retry rather than failing the test.
	var lastErr error
	for attempt := range 20 {
		if _, err := conn.Exec(ctx, stmt); err == nil {
			lastErr = nil
			break
		} else {
			lastErr = err
		}
		time.Sleep(time.Duration(attempt+1) * 25 * time.Millisecond)
	}
	if lastErr != nil {
		return "", fmt.Errorf("create database %q: %w", name, lastErr)
	}

	u, err := url.Parse(admin)
	if err != nil {
		return "", fmt.Errorf("parse connection string: %w", err)
	}
	u.Path = "/" + name

	return u.String(), nil
}

// Template applies a migration once and hands out copies of the result.
//
// Applying a schema per test is the expensive part once the container is
// shared: the projects store replays thirty pgroll migrations for each of its
// nineteen setups. Postgres can copy a prepared database instead, so the
// migration runs once and every caller after that pays a file copy.
type Template struct {
	migrate func(ctx context.Context, dsn string) error

	once sync.Once
	name string
	err  error
}

// NewTemplate returns a Template that prepares a database with migrate. The
// function must close everything it opens: Postgres refuses to copy a database
// that still has sessions attached.
func NewTemplate(migrate func(ctx context.Context, dsn string) error) *Template {
	return &Template{migrate: migrate}
}

// DSN returns a connection string for a fresh database with the template's
// migration already applied.
func (tpl *Template) DSN(ctx context.Context, t *testing.T) string {
	t.Helper()

	tpl.once.Do(func() {
		name := fmt.Sprintf("template_%d", databases.Add(1))
		dsn, err := createDatabase(ctx, name, "")
		if err != nil {
			tpl.err = err
			return
		}
		if err := tpl.migrate(ctx, dsn); err != nil {
			tpl.err = fmt.Errorf("apply template migration: %w", err)
			return
		}
		tpl.name = name
	})
	require.NoError(t, tpl.err, "prepare template database")

	dsn, err := createDatabase(ctx, fmt.Sprintf("test_%d", databases.Add(1)), tpl.name)
	require.NoError(t, err, "copy template database")

	return dsn
}
