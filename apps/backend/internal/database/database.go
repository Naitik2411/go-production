package database

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	config "github.com/Naitik2411/go-tasker/internal/config"
	loggerpkg "github.com/Naitik2411/go-tasker/internal/logger"

	pgxzero "github.com/jackc/pgx-zerolog"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
	"github.com/newrelic/go-agent/v3/integrations/nrpgx5"
	"github.com/rs/zerolog"
)

type Database struct {
	Pool *pgxpool.Pool
	log  *zerolog.Logger
}

type multiTracer struct {
	tracers []any
}

const DatabasePingTimeout = 10

// TraceQueryStart chains all query start tracers
func (mt *multiTracer) TraceQueryStart(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TraceQueryStartData,
) context.Context {

	for _, tracer := range mt.tracers {

		if t, ok := tracer.(interface {
			TraceQueryStart(
				context.Context,
				*pgx.Conn,
				pgx.TraceQueryStartData,
			) context.Context
		}); ok {

			ctx = t.TraceQueryStart(ctx, conn, data)
		}
	}

	return ctx
}

// TraceQueryEnd chains all query end tracers
func (mt *multiTracer) TraceQueryEnd(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TraceQueryEndData,
) {

	for _, tracer := range mt.tracers {

		if t, ok := tracer.(interface {
			TraceQueryEnd(
				context.Context,
				*pgx.Conn,
				pgx.TraceQueryEndData,
			)
		}); ok {

			t.TraceQueryEnd(ctx, conn, data)
		}
	}
}

// New creates a new database connection pool
func New(
	cfg *config.Config,
	logger *zerolog.Logger,
	loggerService *loggerpkg.LoggerService,
) (*Database, error) {

	hostPort := net.JoinHostPort(
		cfg.Database.Host,
		strconv.Itoa(cfg.Database.Port),
	)

	// URL encode password safely
	encodedPassword := url.QueryEscape(cfg.Database.Password)

	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s/%s?sslmode=%s",
		cfg.Database.User,
		encodedPassword,
		hostPort,
		cfg.Database.Name,
		cfg.Database.SSLMode,
	)

	pgxPoolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to parse pgx pool config: %w",
			err,
		)
	}

	// --------------------------------------------------
	// New Relic PostgreSQL Instrumentation
	// --------------------------------------------------

	if loggerService != nil &&
		loggerService.GetApplication() != nil {

		pgxPoolConfig.ConnConfig.Tracer =
			nrpgx5.NewTracer()
	}

	// --------------------------------------------------
	// Local Development Query Logging
	// --------------------------------------------------

	if cfg.Primary.Env == "local" {

		globalLevel := logger.GetLevel()

		pgxLogger := loggerpkg.NewPgxLogger(globalLevel)

		localTracer := &tracelog.TraceLog{
			Logger: pgxzero.NewLogger(pgxLogger),
			LogLevel: tracelog.LogLevel(
				loggerpkg.GetPgxTraceLogLevel(globalLevel),
			),
		}

		// Chain New Relic tracer + local tracer
		if pgxPoolConfig.ConnConfig.Tracer != nil {

			pgxPoolConfig.ConnConfig.Tracer = &multiTracer{
				tracers: []any{
					pgxPoolConfig.ConnConfig.Tracer,
					localTracer,
				},
			}

		} else {

			pgxPoolConfig.ConnConfig.Tracer = localTracer
		}
	}

	// --------------------------------------------------
	// Create Pool
	// --------------------------------------------------

	pool, err := pgxpool.NewWithConfig(
		context.Background(),
		pgxPoolConfig,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"failed to create pgx pool: %w",
			err,
		)
	}

	database := &Database{
		Pool: pool,
		log:  logger,
	}

	// --------------------------------------------------
	// Ping Database
	// --------------------------------------------------

	ctx, cancel := context.WithTimeout(
		context.Background(),
		DatabasePingTimeout*time.Second,
	)

	defer cancel()

	if err = pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf(
			"failed to ping database: %w",
			err,
		)
	}

	logger.Info().
		Msg("connected to the database")

	return database, nil
}

// Close shuts down the database pool
func (db *Database) Close() error {

	db.log.Info().
		Msg("closing database connection pool")

	db.Pool.Close()

	return nil
}
