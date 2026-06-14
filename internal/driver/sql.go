package driver

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/radhe-singh/flowtest/internal/config"
	"github.com/radhe-singh/flowtest/internal/engine"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

// GenericSQLDriver executes database operations using database/sql.
// Used for MySQL and SQLite.
type GenericSQLDriver struct {
	mu      sync.Mutex
	db      *sql.DB
	name    string
	dsn     string
	dialect string // "mysql" or "sqlite"
}

// NewMySQLDriver creates a MySQL driver for the given named database.
func NewMySQLDriver(name, dsn string) *GenericSQLDriver {
	nativeDSN, err := config.ParseMySQLDSN(dsn)
	if err != nil {
		// Store raw DSN; will fail on connect with a clear error
		nativeDSN = dsn
	}
	return &GenericSQLDriver{name: name, dsn: nativeDSN, dialect: "mysql"}
}

// NewSQLiteDriver creates a SQLite driver for the given named database.
func NewSQLiteDriver(name, dsn string) *GenericSQLDriver {
	filePath := config.ParseSQLiteDSN(dsn)
	return &GenericSQLDriver{name: name, dsn: filePath, dialect: "sqlite"}
}

func (d *GenericSQLDriver) Name() string { return d.name }

func (d *GenericSQLDriver) Execute(ctx context.Context, stepConfig interface{}, flowCtx *engine.Context, env *config.EnvConfig) (map[string]interface{}, error) {
	db, err := d.getDB(ctx)
	if err != nil {
		return nil, err
	}

	switch cfg := stepConfig.(type) {
	case *config.SeedConfig:
		return d.executeSeed(ctx, db, cfg)
	case *config.DBStepConfig:
		return d.executeQuery(ctx, db, cfg)
	default:
		return nil, fmt.Errorf("%s driver %q: unsupported config type %T", d.dialect, d.name, stepConfig)
	}
}

func (d *GenericSQLDriver) getDB(ctx context.Context) (*sql.DB, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.db != nil {
		return d.db, nil
	}

	if d.dsn == "" {
		return nil, fmt.Errorf("%s driver %q: no DSN configured", d.dialect, d.name)
	}

	db, err := sql.Open(d.dialect, d.dsn)
	if err != nil {
		return nil, fmt.Errorf("%s driver %q: opening connection: %w", d.dialect, d.name, err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("%s driver %q: connection failed (check credentials and host): %w", d.dialect, d.name, err)
	}

	d.db = db
	return db, nil
}

func (d *GenericSQLDriver) executeSeed(ctx context.Context, db *sql.DB, cfg *config.SeedConfig) (map[string]interface{}, error) {
	if cfg.Table == "" {
		return nil, fmt.Errorf("%s driver %q: seed requires a table name", d.dialect, d.name)
	}
	if len(cfg.Data) == 0 {
		return nil, fmt.Errorf("%s driver %q: seed requires data", d.dialect, d.name)
	}

	columns := make([]string, 0, len(cfg.Data))
	for k := range cfg.Data {
		columns = append(columns, k)
	}
	sort.Strings(columns)

	placeholders := make([]string, len(columns))
	values := make([]interface{}, len(columns))
	for i, col := range columns {
		placeholders[i] = "?"
		values[i] = cfg.Data[col]
	}

	// Quote table name to prevent SQL injection
	quotedTable := quoteIdentifier(d.dialect, cfg.Table)

	var query string
	switch d.dialect {
	case "mysql":
		query = fmt.Sprintf(
			"INSERT IGNORE INTO %s (%s) VALUES (%s)",
			quotedTable,
			strings.Join(columns, ", "),
			strings.Join(placeholders, ", "),
		)
	case "sqlite":
		query = fmt.Sprintf(
			"INSERT OR IGNORE INTO %s (%s) VALUES (%s)",
			quotedTable,
			strings.Join(columns, ", "),
			strings.Join(placeholders, ", "),
		)
	}

	_, err := db.ExecContext(ctx, query, values...)
	if err != nil {
		return nil, fmt.Errorf("%s driver %q: seed insert into %s: %w", d.dialect, d.name, cfg.Table, err)
	}

	return map[string]interface{}{
		"seeded": map[string]interface{}{
			"table": cfg.Table,
			"count": 1,
		},
	}, nil
}

func (d *GenericSQLDriver) executeQuery(ctx context.Context, db *sql.DB, cfg *config.DBStepConfig) (map[string]interface{}, error) {
	if cfg.Query == "" {
		return nil, fmt.Errorf("%s driver %q: query is required", d.dialect, d.name)
	}

	rows, err := db.QueryContext(ctx, cfg.Query, cfg.Params...)
	if err != nil {
		return nil, fmt.Errorf("%s driver %q: executing query: %w", d.dialect, d.name, err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("%s driver %q: getting columns: %w", d.dialect, d.name, err)
	}

	var results []interface{}

	for rows.Next() {
		// Create scan targets
		values := make([]interface{}, len(columns))
		scanArgs := make([]interface{}, len(columns))
		for i := range values {
			scanArgs[i] = &values[i]
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return nil, fmt.Errorf("%s driver %q: scanning row: %w", d.dialect, d.name, err)
		}

		row := make(map[string]interface{}, len(columns))
		for i, col := range columns {
			val := values[i]
			// Convert []byte to string for readability
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s driver %q: iterating rows: %w", d.dialect, d.name, err)
	}

	return map[string]interface{}{
		"rows":      results,
		"row_count": len(results),
	}, nil
}

// Close cleans up the database connection.
func (d *GenericSQLDriver) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db != nil {
		d.db.Close()
		d.db = nil
	}
}

// quoteIdentifier quotes a SQL identifier based on the dialect.
func quoteIdentifier(dialect, name string) string {
	switch dialect {
	case "mysql":
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	default:
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
}
