package config

import "testing"

func TestDetectDriver(t *testing.T) {
	tests := []struct {
		dsn     string
		want    DatabaseDriver
		wantErr bool
	}{
		{"postgres://user:pass@host:5432/db", DriverPostgres, false},
		{"postgresql://user:pass@host:5432/db", DriverPostgres, false},
		{"POSTGRES://user:pass@host:5432/db", DriverPostgres, false},
		{"mysql://user:pass@host:3306/db", DriverMySQL, false},
		{"mongodb://user:pass@host:27017/db", DriverMongoDB, false},
		{"mongodb+srv://user:pass@host/db", DriverMongoDB, false},
		{"sqlite:///path/to/db.db", DriverSQLite, false},
		{"sqlite://./test.db", DriverSQLite, false},
		{"/path/to/file.db", DriverSQLite, false},
		{"./local.sqlite", DriverSQLite, false},
		{"unknown://host/db", "", true},
		{"just-a-string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.dsn, func(t *testing.T) {
			got, err := DetectDriver(tt.dsn)
			if tt.wantErr {
				if err == nil {
					t.Errorf("DetectDriver(%q) expected error, got %q", tt.dsn, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("DetectDriver(%q) unexpected error: %v", tt.dsn, err)
			}
			if got != tt.want {
				t.Errorf("DetectDriver(%q) = %q, want %q", tt.dsn, got, tt.want)
			}
		})
	}
}

func TestParseMySQLDSN(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			"mysql://myuser:mypass@localhost:3306/mydb",
			"myuser:mypass@tcp(localhost:3306)/mydb",
		},
		{
			"mysql://root:@localhost/testdb",
			"root:@tcp(localhost:3306)/testdb",
		},
		{
			"mysql://user:pass@host:3307/db?parseTime=true",
			"user:pass@tcp(host:3307)/db?parseTime=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseMySQLDSN(tt.input)
			if err != nil {
				t.Fatalf("ParseMySQLDSN(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseMySQLDSN(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseSQLiteDSN(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"sqlite:///tmp/test.db", "/tmp/test.db"},
		{"sqlite://./local.db", "./local.db"},
		{"plain.db", "plain.db"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseSQLiteDSN(tt.input)
			if got != tt.want {
				t.Errorf("ParseSQLiteDSN(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseMongoDBName(t *testing.T) {
	tests := []struct {
		dsn     string
		want    string
		wantErr bool
	}{
		{"mongodb://user:pass@host:27017/mydb", "mydb", false},
		{"mongodb+srv://user:pass@host/mydb", "mydb", false},
		{"mongodb://host:27017/", "", true},
		{"mongodb://host:27017", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.dsn, func(t *testing.T) {
			got, err := ParseMongoDBName(tt.dsn)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseMongoDBName(%q) expected error, got %q", tt.dsn, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMongoDBName(%q) error: %v", tt.dsn, err)
			}
			if got != tt.want {
				t.Errorf("ParseMongoDBName(%q) = %q, want %q", tt.dsn, got, tt.want)
			}
		})
	}
}

func TestDefaultDatabase(t *testing.T) {
	t.Run("single db", func(t *testing.T) {
		got, err := DefaultDatabase(map[string]string{"db": "postgres://host/db"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "db" {
			t.Errorf("got %q, want %q", got, "db")
		}
	})

	t.Run("no dbs", func(t *testing.T) {
		_, err := DefaultDatabase(map[string]string{})
		if err == nil {
			t.Fatal("expected error for no databases")
		}
	})

	t.Run("multiple dbs", func(t *testing.T) {
		_, err := DefaultDatabase(map[string]string{"a": "x", "b": "y"})
		if err == nil {
			t.Fatal("expected error for multiple databases")
		}
	})
}

func TestRedactDSN(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{
			"postgres with password",
			"postgres://user:secret@localhost:5432/db",
			"postgres://user:***@localhost:5432/db",
		},
		{
			"mysql with password",
			"mysql://root:hunter2@localhost:3306/app",
			"mysql://root:***@localhost:3306/app",
		},
		{
			"mongodb with password",
			"mongodb://admin:p4ssw0rd@host:27017/mydb",
			"mongodb://admin:***@host:27017/mydb",
		},
		{
			"no password",
			"postgres://user@localhost:5432/db",
			"postgres://user@localhost:5432/db",
		},
		{
			"sqlite (no user info)",
			"sqlite://./test.db",
			"sqlite://./test.db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactDSN(tt.dsn)
			if got != tt.want {
				t.Errorf("RedactDSN(%q) = %q, want %q", tt.dsn, got, tt.want)
			}
		})
	}
}

func TestValidateEnv(t *testing.T) {
	t.Run("missing database", func(t *testing.T) {
		cfg := &FlowConfig{
			Name: "test",
			Steps: []Step{
				{Name: "query", DBStep: &DBStepConfig{Database: "mydb", Query: "SELECT 1"}},
			},
		}
		env := &EnvConfig{Databases: map[string]string{}}
		err := ValidateEnv(cfg, env)
		if err == nil {
			t.Fatal("expected error for missing database")
		}
	})

	t.Run("missing kafka", func(t *testing.T) {
		cfg := &FlowConfig{
			Name: "test",
			Steps: []Step{
				{Name: "consume", Kafka: &KafkaConfig{Topic: "events"}},
			},
		}
		env := &EnvConfig{}
		err := ValidateEnv(cfg, env)
		if err == nil {
			t.Fatal("expected error for missing kafka_brokers")
		}
	})

	t.Run("missing redis", func(t *testing.T) {
		cfg := &FlowConfig{
			Name: "test",
			Steps: []Step{
				{Name: "get", Redis: &RedisConfig{Action: "get", Key: "k"}},
			},
		}
		env := &EnvConfig{}
		err := ValidateEnv(cfg, env)
		if err == nil {
			t.Fatal("expected error for missing redis")
		}
	})

	t.Run("all configured", func(t *testing.T) {
		cfg := &FlowConfig{
			Name: "test",
			Steps: []Step{
				{Name: "query", DBStep: &DBStepConfig{Database: "db", Query: "SELECT 1"}},
				{Name: "consume", Kafka: &KafkaConfig{Topic: "events"}},
				{Name: "get", Redis: &RedisConfig{Action: "get", Key: "k"}},
			},
		}
		env := &EnvConfig{
			Databases:    map[string]string{"db": "postgres://host/db"},
			KafkaBrokers: "localhost:9092",
			Redis:        "redis://localhost:6379",
		}
		err := ValidateEnv(cfg, env)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("api only needs no infra", func(t *testing.T) {
		cfg := &FlowConfig{
			Name: "test",
			Steps: []Step{
				{Name: "health", API: &APIConfig{Method: "GET", URL: "http://localhost/health"}},
			},
		}
		env := &EnvConfig{}
		err := ValidateEnv(cfg, env)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
