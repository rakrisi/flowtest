package driver

import (
	"fmt"

	"github.com/rakrisi/flowtest/internal/config"
	"github.com/rakrisi/flowtest/internal/engine"
)

// Closeable represents a driver that holds resources.
type Closeable interface {
	Close()
}

// Registry holds all available drivers and can register them with an engine.
type Registry struct {
	drivers    map[string]engine.Driver
	closeables []Closeable
}

// NewRegistry creates a registry with the default drivers plus database drivers
// for each configured database.
func NewRegistry(databases map[string]string) (*Registry, error) {
	r := &Registry{
		drivers: make(map[string]engine.Driver),
	}

	// Always available (no external deps)
	r.Register(&HTTPDriver{})
	r.Register(&ShellDriver{})

	// Database drivers — one per configured database
	for name, dsn := range databases {
		dbDriver, err := newDBDriver(name, dsn)
		if err != nil {
			return nil, fmt.Errorf("database %q: %w", name, err)
		}
		r.drivers[name] = dbDriver
		if c, ok := dbDriver.(Closeable); ok {
			r.closeables = append(r.closeables, c)
		}
	}

	// Infrastructure drivers (lazy-connect on first use)
	r.Register(&KafkaDriver{})

	rd := &RedisDriver{}
	r.Register(rd)
	r.closeables = append(r.closeables, rd)

	return r, nil
}

// newDBDriver creates the appropriate database driver based on the DSN scheme.
func newDBDriver(name, dsn string) (engine.Driver, error) {
	driverType, err := config.DetectDriver(dsn)
	if err != nil {
		return nil, err
	}

	switch driverType {
	case config.DriverPostgres:
		return NewPostgresDriver(name, dsn), nil
	case config.DriverMySQL:
		return NewMySQLDriver(name, dsn), nil
	case config.DriverSQLite:
		return NewSQLiteDriver(name, dsn), nil
	case config.DriverMongoDB:
		d, err := NewMongoDriver(name, dsn)
		if err != nil {
			return nil, err
		}
		return d, nil
	default:
		return nil, fmt.Errorf("unsupported database driver %q", driverType)
	}
}

// Register adds a driver to the registry.
func (r *Registry) Register(d engine.Driver) {
	r.drivers[d.Name()] = d
}

// RegisterAll registers all drivers with the given engine.
func (r *Registry) RegisterAll(e *engine.Engine) {
	for name, d := range r.drivers {
		e.RegisterDriver(name, d)
	}
}

// Close cleans up all closeable drivers.
func (r *Registry) Close() {
	for _, c := range r.closeables {
		c.Close()
	}
}
