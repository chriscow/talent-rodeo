package database

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"talent-rodeo/assets"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"

	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/lib/pq"
)

type PG struct {
	*sqlx.DB
	logger *slog.Logger
}

// NewFromPG creates a new DB instance from an existing PostgreSQL connection
func NewFromPG(pg *PG, logger *slog.Logger) *DB {
	logger.Debug("creating new DB instance from PostgreSQL connection")
	return &DB{
		dbtx:   pg.DB,
		logger: logger,
	}
}

func NewPG(dsn string, automigrate bool) (*PG, error) {
	logger := slog.Default()
	logger.Debug("initializing PostgreSQL connection", "dsn_prefix", dsn[:10]+"...")

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	logger.Debug("attempting to connect to PostgreSQL")
	db, err := sqlx.ConnectContext(ctx, "postgres", dsn)
	if err != nil {
		logger.Error("failed to connect to PostgreSQL", "error", err)
		return nil, err
	}
	logger.Info("successfully connected to PostgreSQL")

	logger.Debug("configuring connection pool")
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(2 * time.Hour)

	if automigrate {
		logger.Debug("starting database migration process")
		iofsDriver, err := iofs.New(assets.EmbeddedFiles, "migrations")
		if err != nil {
			logger.Error("failed to create migration source", "error", err)
			return nil, err
		}

		logger.Debug("creating migrator instance")
		migrator, err := migrate.NewWithSourceInstance("iofs", iofsDriver, dsn)
		if err != nil {
			logger.Error("failed to create migrator", "error", err)
			return nil, err
		}

		logger.Debug("running migrations")
		err = migrator.Up()
		switch {
		case errors.Is(err, migrate.ErrNoChange):
			logger.Info("no new migrations to apply")
		case err != nil:
			logger.Error("migration failed", "error", err)
			return nil, err
		default:
			logger.Info("migrations completed successfully")
		}
	}

	// Verify connection is still alive
	if err := db.PingContext(ctx); err != nil {
		logger.Error("failed to ping database after setup", "error", err)
		return nil, err
	}

	return &PG{DB: db, logger: logger}, nil
}
