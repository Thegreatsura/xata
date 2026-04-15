package pgroll

import (
	"context"
	"embed"
	"fmt"
	"path"

	"github.com/rs/zerolog/log"
	"github.com/xataio/pgroll/pkg/backfill"
	"github.com/xataio/pgroll/pkg/migrations"
	"github.com/xataio/pgroll/pkg/roll"
	"github.com/xataio/pgroll/pkg/state"
)

type PGRoll struct {
	schema     string
	migrations []migrations.Migration
}

func FromEmbeddedFS(migrationsFS *embed.FS) (*PGRoll, error) {
	migrations, err := readMigrations(nil, migrationsFS, ".")
	if err != nil {
		return nil, err
	}

	return &PGRoll{
		schema:     "public",
		migrations: migrations,
	}, nil
}

// recursively read migration files and parse them into a slice of migrations
func readMigrations(migrationEntries []migrations.Migration, migrationsFS *embed.FS, currentPath string) ([]migrations.Migration, error) {
	// read migration files
	migrationsEntries, err := migrationsFS.ReadDir(currentPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations: %w", err)
	}

	for _, entry := range migrationsEntries {
		entryPath := path.Join(currentPath, entry.Name())
		if entry.IsDir() {
			migrationEntries, err = readMigrations(migrationEntries, migrationsFS, entryPath)
			if err != nil {
				return nil, err
			}
		} else {
			migration, err := migrations.ReadMigration(migrationsFS, entryPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read migration %s: %w", entry.Name(), err)
			}
			migrationEntries = append(migrationEntries, *migration)
		}
	}

	return migrationEntries, nil
}

// LatestVersionSchema returns the schema name of the latest version, to be used in the search path
func (p *PGRoll) LatestVersionSchema(ctx context.Context) string {
	res := p.schema

	if len(p.migrations) > 0 {
		res += "_" + p.migrations[len(p.migrations)-1].Name
	}

	return res
}

func (p *PGRoll) Migrations() []migrations.Migration {
	return p.migrations
}

// ApplyMigrations applies all pending migrations to the database
func (p *PGRoll) ApplyMigrations(ctx context.Context, pgURL string) error {
	logger := log.Ctx(ctx)

	prState, err := state.New(ctx, pgURL, "pgroll")
	if err != nil {
		return fmt.Errorf("failed to create pgroll state: %w", err)
	}
	defer prState.Close()

	pgroll, err := roll.New(ctx, pgURL, "public", prState)
	if err != nil {
		return fmt.Errorf("failed to create pgroll: %w", err)
	}
	defer pgroll.Close()

	if err := pgroll.Init(ctx); err != nil {
		return fmt.Errorf("failed to init pgroll: %w", err)
	}

	latest, err := prState.LatestVersion(ctx, "public")
	if err != nil {
		return fmt.Errorf("failed to get latest version: %w", err)
	}

	var toRun []migrations.Migration
	var lastMigrationName string
	if latest == nil {
		// no migrations have been run yet, run them all
		logger.Info().Msgf("DB is not initialized")
		toRun = p.migrations
	} else {
		// run migrations from the latest version
		logger.Info().Msgf("Latest version of the schema in the DB: %s", *latest)

		var found bool
		for i, migration := range p.migrations {
			if migration.Name == *latest {
				// found the latest migration, run the rest
				found = true
				lastMigrationName = migration.Name
				toRun = p.migrations[i+1:]
			}
		}

		if !found {
			return fmt.Errorf("failed to find latest migration present in the db. latest: %s", *latest)
		}
	}

	// first check if there is an active migration (to be completed)
	completePrev, err := prState.IsActiveMigrationPeriod(ctx, "public")
	if err != nil {
		return fmt.Errorf("failed to check active migration: %w", err)
	}

	// run all new migrations
	logger.Info().Msgf("Running pending %d migrations", len(toRun))

	for _, migration := range toRun {
		if completePrev {
			logger.Info().Msgf("Completing active migration: %s", lastMigrationName)

			// complete the active migration
			if err := pgroll.Complete(ctx); err != nil {
				return fmt.Errorf("failed to complete migration: %w", err)
			}
		}

		// start the new migration
		logger.Info().Msgf("Starting migration %s", migration.Name)
		if err := pgroll.Start(ctx, &migration, backfill.NewConfig()); err != nil {
			return fmt.Errorf("failed to start migration: %w", err)
		}

		// always complete the previous migration
		completePrev = true
		lastMigrationName = migration.Name
	}

	return nil
}
