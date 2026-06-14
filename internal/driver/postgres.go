package driver

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/radhe-singh/flowtest/internal/config"
	"github.com/radhe-singh/flowtest/internal/engine"
)

// PostgresDriver executes database seeds and queries against a PostgreSQL instance.
type PostgresDriver struct {
	mu   sync.Mutex
	pool *pgxpool.Pool
	name string
	dsn  string
}

// NewPostgresDriver creates a PostgreSQL driver for the given named database.
func NewPostgresDriver(name, dsn string) *PostgresDriver {
	return &PostgresDriver{name: name, dsn: dsn}
}

func (d *PostgresDriver) Name() string { return d.name }

func (d *PostgresDriver) Execute(ctx context.Context, stepConfig interface{}, flowCtx *engine.Context, env *config.EnvConfig) (map[string]interface{}, error) {
	pool, err := d.getPool(ctx)
	if err != nil {
		return nil, err
	}

	switch cfg := stepConfig.(type) {
	case *config.SeedConfig:
		return d.executeSeed(ctx, pool, cfg)
	case *config.DBStepConfig:
		return d.executeQuery(ctx, pool, cfg)
	default:
		return nil, fmt.Errorf("postgres driver %q: unsupported config type %T", d.name, stepConfig)
	}
}

func (d *PostgresDriver) getPool(ctx context.Context) (*pgxpool.Pool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.pool != nil {
		return d.pool, nil
	}

	if d.dsn == "" {
		return nil, fmt.Errorf("postgres driver %q: no DSN configured", d.name)
	}

	pool, err := pgxpool.New(ctx, d.dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres driver %q: connecting to %s: %w", d.name, config.RedactDSN(d.dsn), err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres driver %q: pinging %s: %w", d.name, config.RedactDSN(d.dsn), err)
	}

	d.pool = pool
	return pool, nil
}

func (d *PostgresDriver) executeSeed(ctx context.Context, pool *pgxpool.Pool, cfg *config.SeedConfig) (map[string]interface{}, error) {
	if cfg.Table == "" {
		return nil, fmt.Errorf("postgres driver %q: seed requires a table name", d.name)
	}
	if len(cfg.Data) == 0 {
		return nil, fmt.Errorf("postgres driver %q: seed requires data", d.name)
	}

	// Sort columns for deterministic ordering
	columns := make([]string, 0, len(cfg.Data))
	for k := range cfg.Data {
		columns = append(columns, k)
	}
	sort.Strings(columns)

	placeholders := make([]string, len(columns))
	values := make([]interface{}, len(columns))
	for i, col := range columns {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		values[i] = cfg.Data[col]
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING",
		pgx.Identifier{cfg.Table}.Sanitize(),
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	_, err := pool.Exec(ctx, query, values...)
	if err != nil {
		return nil, fmt.Errorf("postgres driver %q: seed insert into %s: %w", d.name, cfg.Table, err)
	}

	return map[string]interface{}{
		"seeded": map[string]interface{}{
			"table": cfg.Table,
			"count": 1,
		},
	}, nil
}

func (d *PostgresDriver) executeQuery(ctx context.Context, pool *pgxpool.Pool, cfg *config.DBStepConfig) (map[string]interface{}, error) {
	if cfg.Query == "" {
		return nil, fmt.Errorf("postgres driver %q: query is required", d.name)
	}

	rows, err := pool.Query(ctx, cfg.Query, cfg.Params...)
	if err != nil {
		return nil, fmt.Errorf("postgres driver %q: executing query: %w", d.name, err)
	}
	defer rows.Close()

	fieldDescs := rows.FieldDescriptions()
	var results []interface{}

	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("postgres driver %q: scanning row: %w", d.name, err)
		}

		row := make(map[string]interface{}, len(fieldDescs))
		for i, fd := range fieldDescs {
			row[string(fd.Name)] = values[i]
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres driver %q: iterating rows: %w", d.name, err)
	}

	return map[string]interface{}{
		"rows":      results,
		"row_count": len(results),
	}, nil
}

// Close cleans up the connection pool.
func (d *PostgresDriver) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pool != nil {
		d.pool.Close()
		d.pool = nil
	}
}
