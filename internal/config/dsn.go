package config

import (
	"fmt"
	"net/url"
	"strings"
)

// DatabaseDriver represents a supported database type.
type DatabaseDriver string

const (
	DriverPostgres DatabaseDriver = "postgres"
	DriverMySQL    DatabaseDriver = "mysql"
	DriverMongoDB  DatabaseDriver = "mongodb"
	DriverSQLite   DatabaseDriver = "sqlite"
)

// DetectDriver determines the database driver from a DSN string.
// Supported schemes: postgres://, postgresql://, mysql://, mongodb://, mongodb+srv://, sqlite://.
// File paths ending in .db or .sqlite are treated as SQLite.
func DetectDriver(dsn string) (DatabaseDriver, error) {
	lower := strings.ToLower(dsn)

	switch {
	case strings.HasPrefix(lower, "postgres://"), strings.HasPrefix(lower, "postgresql://"):
		return DriverPostgres, nil
	case strings.HasPrefix(lower, "mysql://"):
		return DriverMySQL, nil
	case strings.HasPrefix(lower, "mongodb://"), strings.HasPrefix(lower, "mongodb+srv://"):
		return DriverMongoDB, nil
	case strings.HasPrefix(lower, "sqlite://"):
		return DriverSQLite, nil
	case strings.HasSuffix(lower, ".db"), strings.HasSuffix(lower, ".sqlite"):
		return DriverSQLite, nil
	default:
		return "", fmt.Errorf("cannot detect database driver from DSN %q — use a supported scheme (postgres://, mysql://, mongodb://, sqlite://)", dsn)
	}
}

// ParseMySQLDSN converts a mysql:// URL into go-sql-driver/mysql format.
// mysql://user:pass@host:port/dbname?params → user:pass@tcp(host:port)/dbname?params
func ParseMySQLDSN(dsn string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parsing MySQL DSN: %w", err)
	}

	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "3306"
	}

	password, _ := u.User.Password()
	user := u.User.Username()

	dbName := strings.TrimPrefix(u.Path, "/")

	result := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", user, password, host, port, dbName)

	if u.RawQuery != "" {
		result += "?" + u.RawQuery
	}

	return result, nil
}

// ParseSQLiteDSN extracts the file path from a sqlite:// URL.
// sqlite:///path/to/db.db → /path/to/db.db
// sqlite://./relative.db → ./relative.db
// plain.db → plain.db (passthrough)
func ParseSQLiteDSN(dsn string) string {
	if strings.HasPrefix(strings.ToLower(dsn), "sqlite://") {
		path := dsn[len("sqlite://"):]
		return path
	}
	return dsn
}

// ParseMongoDBName extracts the database name from a mongodb:// URL.
// mongodb://user:pass@host:port/dbname → dbname
func ParseMongoDBName(dsn string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parsing MongoDB DSN: %w", err)
	}

	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		return "", fmt.Errorf("MongoDB DSN must include a database name in the path: %s", dsn)
	}

	// Remove any query part from the path
	if idx := strings.Index(dbName, "?"); idx != -1 {
		dbName = dbName[:idx]
	}

	return dbName, nil
}

// RedactDSN masks the password in a DSN URL for safe logging.
// postgres://user:secret@host → postgres://user:***@host
func RedactDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "***"
	}
	if u.User == nil {
		return dsn
	}
	if _, hasPass := u.User.Password(); !hasPass {
		return dsn
	}
	// Rebuild manually to avoid URL-encoding the asterisks
	return strings.Replace(dsn, u.User.String()+"@", u.User.Username()+":***@", 1)
}

// DefaultDatabase returns the name of the first database if there is exactly one,
// or an error if there are zero or multiple databases and no target is specified.
func DefaultDatabase(databases map[string]string) (string, error) {
	if len(databases) == 0 {
		return "", fmt.Errorf("no databases configured")
	}
	if len(databases) == 1 {
		for name := range databases {
			return name, nil
		}
	}
	return "", fmt.Errorf("multiple databases configured — specify target database explicitly")
}
