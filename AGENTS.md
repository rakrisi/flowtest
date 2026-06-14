# AGENTS.md — FlowTest 

## Project Overview

FlowTest is a Go CLI tool that reads YAML flow files and executes backend integration test flows: DB seed, API calls (GET/POST/PUT/PATCH/DELETE), Kafka event search, Redis cache validation, shell scripts — chained together with variable passing and expr-lang assertions.

Supports **multiple database engines**: PostgreSQL, MySQL, MongoDB, and SQLite — auto-detected from DSN schemes.

**Module:** `github.com/rakrisi/flowtest`
**Go Version:** 1.25.0
**Binary:** `flowtest`
**Version:** 1.0.0

---

## Quick Reference

```bash
# Build
go build -o flowtest ./cmd/flowtest/

# Test
go test ./...

# Vet
go vet ./...

# Run a flow
./flowtest run flows/smoke-test.yaml -v

# Validate a flow
./flowtest validate flows/example.yaml --check

# Dry-run (show plan without executing)
./flowtest run flows/example.yaml --dry-run

# Integration tests (requires Docker)
cd testproject && ./run-tests.sh
```

---

## Directory Structure

```
backflow/
├── cmd/flowtest/
│   └── main.go                          # Cobra CLI: run, list, validate, init
├── internal/
│   ├── config/
│   │   ├── schema.go                    # All YAML type definitions
│   │   ├── loader.go                    # YAML/JSON parsing, env resolution, validation
│   │   ├── loader_test.go               # 12 tests (loading, validation, merging, DB steps)
│   │   ├── dsn.go                       # DSN parsing: detect driver, parse MySQL/SQLite/MongoDB
│   │   └── dsn_test.go                  # 5 tests (driver detection, DSN parsing)
│   ├── engine/
│   │   ├── engine.go                    # Step loop, driver dispatch, setup/cleanup, retry, dry-run
│   │   ├── context.go                   # Variable store, ${var} resolution, expr evaluation
│   │   ├── result.go                    # StepResult, FlowResult, StepDetail, status tallying
│   │   ├── engine_test.go               # 8 tests (mock drivers, fail-fast, variable chaining, DB steps)
│   │   ├── context_test.go              # 5 tests (set/get, resolve, eval, save)
│   │   └── result_test.go              # 2 tests (tally, success)
│   ├── driver/
│   │   ├── registry.go                  # Driver registration, DSN-based DB driver creation, cleanup
│   │   ├── http.go                      # HTTP driver (all methods, headers, JSON body/response)
│   │   ├── postgres.go                  # Postgres driver (pgx/v5, seed + parameterized query)
│   │   ├── mysql.go                     # MySQL driver constructor (delegates to sql.go)
│   │   ├── sqlite.go                    # SQLite driver constructor (delegates to sql.go)
│   │   ├── sql.go                       # GenericSQLDriver — shared implementation for MySQL/SQLite
│   │   ├── mongo.go                     # MongoDB driver (find/findOne/insert/update/delete/count)
│   │   ├── kafka.go                     # Kafka driver (produce + direct partition search)
│   │   ├── redis.go                     # Redis driver (get/set/del/exists/hgetall/keys/ttl)
│   │   └── shell.go                     # Shell driver (exec, env injection, exit code assert)
│   └── output/
│       ├── printer.go                   # TerminalPrinter (colored, verbose), NullPrinter (JSON mode)
│       └── summary.go                   # Pass/fail/skip counts + JSON output
├── flows/                               # Example flow files
│   ├── example.yaml                     # Starter template (health + echo)
│   ├── smoke-test.yaml                  # Engine validation (shell driver, variable passing)
│   ├── http-test.yaml                   # HTTP driver against httpbin.org
│   ├── order-flow.yaml                  # Full lifecycle reference (DB->API->Kafka->Redis)
│   └── auth-flow.yaml                   # Auth lifecycle reference (register->login->token)
├── testproject/                         # Integration test suite
│   ├── main.go                          # Test API server (Go, CRUD + Kafka + Redis)
│   ├── docker-compose.yaml              # Postgres 16, MySQL 8, MongoDB 7, Redis 7, Kafka 7.5
│   ├── init.sql                         # Postgres schema: items + users tables
│   ├── init-mysql.sql                   # MySQL schema
│   ├── flowtest.yaml                    # Test config with all connection strings
│   ├── run-tests.sh                     # One-command: docker up -> build -> run all flows
│   ├── go.mod / go.sum
│   └── flows/
│       ├── 01-health-and-db-seed.yaml   # Health check + DB seed/query
│       ├── 02-api-crud.yaml             # POST/GET/PUT/PATCH/DELETE + DB verify (11 steps)
│       ├── 03-redis-validation.yaml     # Cache + SET/GET/EXISTS/TTL/KEYS/DEL (12 steps)
│       ├── 04-kafka-validation.yaml     # created/updated/patched/deleted events (8 steps)
│       ├── 05-full-integration.yaml     # End-to-end: seed->API->Kafka->Redis->DB (15 steps)
│       ├── 06-mysql-validation.yaml     # MySQL seed, query, CRUD via API
│       ├── 07-mongo-validation.yaml     # MongoDB insertOne/find/updateOne/deleteOne/count
│       ├── 08-sqlite-validation.yaml    # SQLite seed, query, CRUD operations
│       └── 09-multi-db-integration.yaml # Cross-database operations
├── flowtest.yaml                        # Global config template
├── go.mod / go.sum
├── AGENTS.md                            # This file
└── README.md                            # Project README
```

---

## Architecture

### Core Engine (`internal/engine/`)

The engine is the brain. It does NOT know about HTTP, Kafka, or Postgres.

**Execution flow:**
1. Create `Context` (empty variable store)
2. Seed with `--var` flags if provided
3. Run `setup` steps (seed/db)
4. For each step:
   - Wait `delay` if configured
   - Evaluate `when` condition via expr-lang (skip if false)
   - Resolve `${var}` placeholders in step config
   - Dispatch to the correct driver by step type
   - If `--dry-run`: show plan and skip execution
   - Merge driver result into context
   - Run `assert` expressions against context
   - Execute `save` mappings (only if assertions pass)
   - Record StepResult (passed/failed/skipped/errored + duration)
   - If `retry` configured and failed: retry with backoff
5. Run `cleanup` steps (always, even on failure)
6. Tally and return FlowResult

**Key types:**

```go
// Driver interface — all drivers implement this
type Driver interface {
    Name() string
    Execute(ctx context.Context, stepConfig interface{}, flowCtx *Context, env *config.EnvConfig) (map[string]interface{}, error)
}

// Printer interface — pluggable output
type Printer interface {
    FlowHeader(name string)
    SectionHeader(name string)
    StepStart(name string, driverType string, stepNum int, totalSteps int)
    StepResult(result *StepResult, stepNum int, totalSteps int, verbose bool)
    SetupStart(description string)
    SetupResult(description string, err error)
    CleanupStart(description string)
    CleanupResult(description string, err error)
}
```

**Variable resolution:**
- `${var}` in strings replaced from context
- Dot notation supported: `${response.body.user.id}`
- `ResolveInterface()` handles nested maps, slices, strings recursively
- System env vars `${UPPER_CASE}` resolved at YAML load time
- Flow variables `${lower_case}` resolved at step execution time

**Expression evaluation:**
- Uses `expr-lang/expr` library
- Evaluated against `context.All()` map (all saved variables + driver results)
- IMPORTANT: `count` is a reserved word in expr-lang; use `key_count` for Redis KEYS results

### Config Layer (`internal/config/`)

**Two config files:**
- `flowtest.yaml` — global config (connection strings, profiles)
- `flows/*.yaml` — individual flow definitions

**Merge priority:** profile > flow-level > global

**DSN auto-detection** (`dsn.go`):
- `postgres://` or `postgresql://` → PostgreSQL driver
- `mysql://` → MySQL driver (converted to go-sql-driver format)
- `mongodb://` or `mongodb+srv://` → MongoDB driver
- `sqlite://` or `.db`/`.sqlite` file extension → SQLite driver

**Key types:**

| Type | Fields |
|------|--------|
| `FlowConfig` | name, description, timeout, fail_fast, env, setup, cleanup, steps |
| `EnvConfig` | api_base, databases (map[string]string), kafka_brokers, redis |
| `Step` | name, when, delay, retry, api/DBStep/kafka/redis/script (exactly one), assert, save |
| `RetryConfig` | times, interval, backoff ("linear" or "exponential") |
| `APIConfig` | method, url, headers, auth, body, timeout |
| `AuthConfig` | bearer, basic, api_key (only one should be set) |
| `BasicAuthConfig` | username, password |
| `APIKeyConfig` | header OR query, value |
| `DBStepConfig` | database, query, params (SQL) OR collection, operation, filter, document, update (MongoDB) |
| `SeedConfig` | target, table, data |
| `KafkaConfig` | action, topic, timeout, match, key, message, headers |
| `RedisConfig` | action, key, value, ttl |
| `ScriptConfig` | lang, run, assert_exit, timeout |
| `AssertConfig` | expr, msg |

**`Step.DriverType()` mapping:**
- `api` field set -> `"http"` driver
- DB field set -> database name (e.g. `"db"`, `"mysql_db"`, `"mongo_db"`)
- `kafka` field set -> `"kafka"` driver
- `redis` field set -> `"redis"` driver
- `script` field set -> `"shell"` driver

**Custom YAML unmarshaling:**
Steps use custom `UnmarshalYAML` to detect dynamic database keys. Any YAML key not in the known set (`name`, `when`, `delay`, `retry`, `api`, `kafka`, `redis`, `script`, `assert`, `save`) is treated as a database name and decoded into `DBStepConfig`. Same pattern applies to `SetupStep` and `CleanupStep`.

### Drivers (`internal/driver/`)

All drivers are registered in `registry.go` at startup. The registry auto-creates database drivers from DSN schemes. Infrastructure drivers (Postgres, MySQL, SQLite, MongoDB, Redis) use lazy connections on first use.

#### HTTP Driver (`"http"`)

**Accepts:** `*config.APIConfig`
**Returns:**
```
response.status    int        HTTP status code
response.headers   map        Response headers
response.body      any        Parsed JSON body (or raw string)
```
- Auto-prepends `env.api_base` if URL is relative
- **Auth support:** Bearer token, Basic auth, API key (header or query param)
- Auto-sets `Content-Type: application/json` when body present
- Default timeout: 30s

#### PostgreSQL Driver (named by database key)

**Accepts:** `*config.DBStepConfig` or `*config.SeedConfig`
**Returns (query):**
```
rows         []map[string]interface{}   Query result rows
row_count    int                        Number of rows returned
```
**Returns (seed):**
```
seeded.table    string    Table name
seeded.count    int       Rows inserted
```
- Uses `pgx/v5` with connection pooling
- Seed uses `INSERT ... ON CONFLICT DO NOTHING`
- Query params use `$1, $2, ...` PostgreSQL positional syntax

#### MySQL/SQLite Driver (named by database key)

**Shared implementation via `GenericSQLDriver`** in `sql.go`.

**Accepts:** `*config.DBStepConfig` or `*config.SeedConfig`
**Returns:** Same as PostgreSQL (`rows`, `row_count`, `seeded`)
- Uses `database/sql` with `go-sql-driver/mysql` or `modernc.org/sqlite`
- Seed uses `INSERT IGNORE` (MySQL) or `INSERT OR IGNORE` (SQLite)
- Query params use `?` placeholder syntax
- MySQL DSNs converted from URL format to go-sql-driver native format

#### MongoDB Driver (named by database key)

**Accepts:** `*config.DBStepConfig`

**Supported operations:**
| Operation | Fields | Returns |
|-----------|--------|---------|
| `find` | filter | `rows` ([]docs), `row_count` |
| `findOne` | filter | `rows` (0 or 1 doc), `row_count` |
| `insertOne` | document | `rows` ([{_id}]), `row_count` = 1 |
| `insertMany` | documents | `rows` ([{_id}...]), `row_count` |
| `updateOne` | filter, update | `row_count` (matched), `modified_count` |
| `deleteOne` | filter | `row_count` (deleted) |
| `deleteMany` | filter | `row_count` (deleted) |
| `countDocuments` | filter | `row_count` |

- Uses `go.mongodb.org/mongo-driver/v2`
- Auto-wraps update in `$set` if no update operator present
- BSON documents normalized to `map[string]interface{}`

#### Kafka Driver (`"kafka"`)

**Accepts:** `*config.KafkaConfig`

**Search/consume (default action) returns:**
```
message.topic       string    Topic name
message.partition   int       Partition number
message.offset      int64     Message offset
message.key         string    Message key
message.timestamp   int64     Unix timestamp
message.payload     any       Parsed JSON payload (or raw string)
message.headers     map       Message headers (if present)
```

**Produce returns:**
```
produced.topic    string    Topic name
produced.key      string    Message key
```

**Architecture:** Direct partition read — no consumer groups, no side effects.
- Connects to each partition, reads from `FirstOffset`, scans forward
- Matches messages against `match` filter (compares JSON payload fields)
- Times out if no match found within `timeout` (default 10s)
- IMPORTANT: Always use `match` filter to find the correct message

#### Redis Driver (`"redis"`)

**Accepts:** `*config.RedisConfig`

| Action | Returns |
|--------|---------|
| `get` | `value` (auto-parsed JSON or string), `exists` (bool), `raw` (string) |
| `set` | `ok` (bool) |
| `exists` | `exists` (bool), `value` (bool) |
| `hgetall` | `value` (map), `exists` (bool) |
| `keys` | `value` ([]string), `key_count` (int) — NOTE: `key_count` not `count` |
| `del` | `deleted` (int) |
| `ttl` | `ttl` (float64 seconds), `expires` (bool) |

- Auto-parses JSON strings into objects for `get` and `hgetall`
- IMPORTANT: Redis KEYS returns `key_count` (not `count`) because `count` is reserved in expr-lang

#### Shell Driver (`"shell"`)

**Accepts:** `*config.ScriptConfig`
**Returns:**
```
stdout       string    Trimmed stdout
stderr       string    Trimmed stderr
exit_code    int       Process exit code
```
- Default shell: `/bin/sh` (use `lang: bash` for bash)
- Context variables injected as `FLOWTEST_<UPPER_KEY>` env vars
- `assert_exit` checked before returning; error on mismatch
- Default timeout: 30s

### Output (`internal/output/`)

Two implementations of `engine.Printer`:
- `TerminalPrinter` — colored output: green check (pass), red X (fail), yellow dash (skip), red ! (error)
- `NullPrinter` — silent (used when `--output-json` is set)

`PrintSummary()` — final line: `N passed  N failed  N skipped  (duration)`
`PrintJSONResult()` — full FlowResult as indented JSON to stdout

Verbose mode shows driver-specific details: HTTP request/response, SQL queries with params, MongoDB collection.operation with filter, Kafka topic/action/match, Redis action/key, shell command/stdout, and saved variables.

---

## CLI Commands

### `flowtest run <flow-file>`

Execute a flow.

| Flag | Description |
|------|-------------|
| `-p, --profile STRING` | Environment profile from flowtest.yaml |
| `-v, --verbose` | Show request/response details, queries, assertions |
| `--fail-fast` | Stop on first failure |
| `--output-json` | Output FlowResult as JSON (suppresses terminal output) |
| `--from-json FILE` | Load flow from JSON instead of YAML |
| `--var key=value` | Inject initial variables (repeatable) |
| `--step N` | Start from step N (skip earlier steps) |
| `--step-name STRING` | Start from the named step (case-insensitive) |
| `--dry-run` | Show what would execute without running anything |

Exit code: 0 if all passed/skipped, 1 if any failed/errored.

### `flowtest validate <flow-file>`

Parse and validate without executing.

| Flag | Description |
|------|-------------|
| `--check` | TCP connectivity check to configured services |
| `-p, --profile STRING` | Environment profile |

### `flowtest list [dir]`

List all `.yaml`/`.yml` files in `flows/` (or given dir) with flow names.

### `flowtest init`

Scaffold `flowtest.yaml` and `flows/example.yaml`.

---

## YAML Flow Schema

```yaml
name: Flow Name                          # REQUIRED
description: Optional description
timeout: 30s                             # Optional global timeout
fail_fast: true                          # Optional, stop on first failure

env:                                     # Optional, overrides global config
  api_base: http://localhost:8000
  databases:                             # Named database connections
    db: postgres://user:pass@host:5432/dbname
    mysql_db: mysql://user:pass@host:3306/dbname
    mongo_db: mongodb://host:27017/dbname
    local: sqlite://./file.db
  kafka_brokers: localhost:9092
  redis: redis://localhost:6379

setup:                                   # Optional, runs before steps
  - seed:
      target: db                         # Optional, defaults to first DB
      table: users
      data: {email: test@test.com, name: Test}
  - db:                                  # Database key name
      query: "DELETE FROM temp_data"

cleanup:                                 # Optional, runs after steps (always)
  - db:
      query: "DELETE FROM items WHERE name LIKE 'test%'"

steps:                                   # REQUIRED, at least one
  - name: Step Name                      # REQUIRED
    when: "condition_expr"               # Optional, skip if false
    delay: 1s                            # Optional, wait before executing
    retry:                               # Optional, retry on failure
      times: 3                           # Max attempts
      interval: 500ms                    # Wait between retries
      backoff: exponential               # "linear" (default) or "exponential"

    # Exactly one driver config per step:

    api:                                 # HTTP driver
      method: POST
      url: /endpoint
      headers:                           # Custom headers (optional)
        X-Request-ID: req-123
        X-Custom: "custom-value"
      auth:                              # Optional authentication
        bearer: ${token}                 # Bearer token auth
        # OR
        basic:                           # Basic auth
          username: admin
          password: ${password}
        # OR
        api_key:                         # API key auth
          header: X-API-Key              # or use "query" for query param
          value: ${api_key}
      body:                              # Supports any JSON structure
        simple_field: value
        nested:                          # Nested objects
          user:
            name: ${user_name}
            address:
              city: ${city}
        items:                           # Arrays of objects
          - id: 1
            name: Item One
          - id: 2
            name: Item Two
        tags: [urgent, priority]         # Simple arrays
      timeout: 10s

    db:                                  # SQL database (name matches databases key)
      query: "SELECT * FROM users WHERE id = $1"
      params: [${user_id}]

    mongo_db:                            # MongoDB (name matches databases key)
      collection: users
      operation: find                    # find, findOne, insertOne, insertMany, updateOne, deleteOne, deleteMany, countDocuments
      filter: {email: "${email}"}
      document: {name: "Test"}           # for insertOne
      documents: [{a: 1}, {b: 2}]       # for insertMany
      update: {$set: {status: active}}   # for updateOne

    kafka:                               # Kafka driver
      action: produce                    # "produce" or "consume" (default)
      topic: events
      key: event-key
      message: {status: created}
      headers: {event-type: order.created}
      match: {order_id: ${order_id}}     # for consume: filter to find message
      timeout: 5s

    redis:                               # Redis driver
      action: get                        # get, set, del, keys, exists, ttl, hgetall
      key: cache:${user_id}
      value: ${data}                     # for set
      ttl: 1h                            # for set

    script:                              # Shell driver
      lang: bash
      run: echo "hello"
      assert_exit: 0
      timeout: 30s

    assert:                              # Optional
      - expr: "response.status == 201"
        msg: "Optional failure message"
    save:                                # Optional, only runs if assertions pass
      var_name: response.body.field
```

---

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `gopkg.in/yaml.v3` | YAML parsing |
| `github.com/expr-lang/expr` | Expression evaluation for assertions |
| `github.com/jackc/pgx/v5` | PostgreSQL driver with connection pooling |
| `github.com/go-sql-driver/mysql` | MySQL driver |
| `modernc.org/sqlite` | SQLite driver (pure Go, no CGO) |
| `go.mongodb.org/mongo-driver/v2` | MongoDB client |
| `github.com/segmentio/kafka-go` | Kafka client (direct partition read + writer) |
| `github.com/redis/go-redis/v9` | Redis client |
| `github.com/fatih/color` | Terminal color output |

---

## Testing

### Unit Tests (32 top-level tests, 90 subtests)

```bash
go test ./... -v
```

| Package | Tests | Covers |
|---------|-------|--------|
| `internal/config` | 17 | Loading, validation, env var resolution, profile merging, JSON input, DriverType, DSN detection, MySQL/SQLite/MongoDB DSN parsing, DB step unmarshaling |
| `internal/engine` | 15 | Context set/get/resolve/eval/save, engine flow execution, fail-fast, when conditions, variable chaining, driver errors, initial variables, DB step dispatch, result tallying |

Tests use mock drivers and table-driven patterns. No external dependencies required.

### Integration Tests (testproject/)

```bash
cd testproject && ./run-tests.sh
```

**Requires:** Docker running (Postgres, MySQL, MongoDB, Redis, Kafka, Zookeeper)

**Connection strings:**
- Postgres: `postgres://myuser:mypassword@localhost:5432/referral?sslmode=disable`
- MySQL: `mysql://myuser:mypassword@localhost:3306/testdb`
- MongoDB: `mongodb://localhost:27017/testdb`
- SQLite: `sqlite://./test.db`
- Redis: `redis://localhost:6379`
- Kafka: `localhost:9092`
- API: `http://localhost:8000`

**Test server** (`testproject/main.go`): Go HTTP server with CRUD on `/items`, Kafka event publishing, Redis caching, auth endpoints, echo endpoint.

**11 flow files, 95+ steps total:**

| Flow | Steps | Covers |
|------|-------|--------|
| 01-health-and-db-seed | 4 | Health endpoint, DB INSERT with params, DB SELECT verify |
| 02-api-crud | 11 | POST/GET/PUT/PATCH/DELETE, DB state verification, 404 after delete |
| 03-redis-validation | 12 | Cache hit, EXISTS, TTL, SET, GET, KEYS, DEL, cache invalidation |
| 04-kafka-validation | 8 | item.created, item.updated, item.patched, item.deleted events |
| 05-full-integration | 15 | End-to-end: seed->API->Kafka->Redis->DB->cleanup with fail_fast |
| 06-mysql-validation | — | MySQL seed, query, CRUD operations via API |
| 07-mongo-validation | — | MongoDB insertOne, find, findOne, updateOne, deleteOne, countDocuments |
| 08-sqlite-validation | — | SQLite seed, query, CRUD operations |
| 09-multi-db-integration | — | Cross-database operations: Postgres + MySQL + MongoDB + SQLite |
| 10-auth-validation | 16 | Bearer, Basic, API key (header/query) auth, variable interpolation |
| 11-advanced-body-response | 9 | Nested objects, arrays, multi-level nesting, variable interpolation, custom headers |

---

## Code Conventions

### Error Handling
- All errors wrapped with context: `fmt.Errorf("driver_name: operation: %w", err)`
- No silent failures — all errors surface to the user
- Validation errors collected and reported together
- Step errors marked as `StatusErrored`, assertion failures as `StatusFailed`

### Naming
- Driver names: match database key from config (e.g. `"db"`, `"mysql_db"`, `"mongo_db"`) or fixed (`"http"`, `"kafka"`, `"redis"`, `"shell"`)
- Result keys: snake_case (`exit_code`, `row_count`, `key_count`, `modified_count`)
- Config fields: CamelCase Go structs with `yaml:"snake_case"` tags
- Packages: singular (`config`, `engine`, `driver`, `output`)

### Import Organization
```go
import (
    "standard/library"

    "external/dependency"

    "github.com/rakrisi/flowtest/internal/..."
)
```

### Concurrency
- Postgres, MySQL, SQLite, MongoDB, and Redis drivers use `sync.Mutex` for lazy connection pooling
- Kafka search spawns one goroutine per partition, coordinates via channel
- Context cancellation propagated through `context.Context`
- Shell driver uses `context.WithTimeout` for command execution

### Interface Design
- Small, focused interfaces: `Driver`, `Printer`, `Closeable`
- Dependency injection via constructors and method parameters
- No global state — driver registration via `Registry`

---

## Known Gotchas

1. **`count` is reserved in expr-lang** — Redis KEYS returns `key_count`. Any variable named `count` will shadow the built-in `count()` function.

2. **Kafka search reads from beginning** — Uses `FirstOffset` on all partitions. Always use `match` filters to find the correct message, especially on topics that accumulate events across test runs.

3. **DB params need `${var}` resolution** — The engine resolves `${var}` in `params` arrays. Use PostgreSQL `$1, $2` positional syntax for Postgres, `?` for MySQL/SQLite. Pass flow variables via params.

4. **JSON number types** — All JSON numbers unmarshal as `float64` in Go. Assertions like `response.body.id == 5` compare `float64` to `int`. expr-lang handles this, but be aware.

5. **HTTP body implies Content-Type** — Setting `body` auto-adds `Content-Type: application/json`. To send non-JSON, don't use `body`.

6. **`save` only runs if assertions pass** — If any assertion fails, save mappings are skipped. This prevents propagating bad data to later steps.

7. **Setup/cleanup always run** — Cleanup runs even if steps fail or `fail_fast` is enabled.

8. **System vs flow variables** — `${UPPER_CASE}` resolved from OS environment at YAML load time. `${lower_case}` resolved from flow context at step execution time.

9. **Database keys are dynamic YAML keys** — Step driver type for databases is determined by the YAML key name (e.g. `db:`, `mysql_db:`, `mongo_db:`), not a fixed keyword. The key must match a name in the `databases` config map.

10. **MongoDB auto-wraps update** — If `update` map doesn't contain a MongoDB operator (`$set`, `$unset`), it's automatically wrapped in `$set`.

11. **Seed target defaults** — `seed.target` is optional; defaults to the first (and only) database. If multiple databases are configured, `target` is required.
