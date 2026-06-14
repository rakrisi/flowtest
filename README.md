# FlowTest

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev)
[![Release](https://img.shields.io/github/v/release/rakrisi/flowtest?style=flat)](https://github.com/rakrisi/flowtest/releases)
[![License](https://img.shields.io/badge/License-MIT-blue.svg?style=flat)](LICENSE)
[![Tests](https://img.shields.io/badge/tests-passing-brightgreen?style=flat)](https://github.com/rakrisi/flowtest/actions)
[![Coverage](https://img.shields.io/badge/coverage-61%25-yellow?style=flat)]()
[![Go Report Card](https://goreportcard.com/badge/github.com/rakrisi/flowtest)](https://goreportcard.com/report/github.com/rakrisi/flowtest)

Declarative backend integration testing. Write YAML, test everything.

FlowTest reads YAML flow files and executes backend test sequences — API calls, database operations, Kafka events, Redis validation, and shell scripts — with variable passing, conditional execution, and expression-based assertions.

## In 60 seconds

```bash
# 1. Install
curl -sSfL https://raw.githubusercontent.com/rakrisi/flowtest/main/install.sh | sh

# 2. Create a flow (no infrastructure required)
cat > hello.yaml << 'EOF'
name: Hello FlowTest
steps:
  - name: Script works
    script:
      run: echo "FlowTest is working!"
      assert_exit: 0
    assert:
      - expr: "stdout == 'FlowTest is working!'"
EOF

# 3. Run it
flowtest run hello.yaml
```

```
  ✓ Script works  (12ms)

  Passed 1/1 steps
```

Once your API/DB is running, scaffold a real project:

```bash
mkdir my-tests && cd my-tests
flowtest init          # interactive: picks only the services you use
flowtest run flows/example.yaml
```

## Features

- **Multi-database support** — PostgreSQL, MySQL, MongoDB, SQLite (auto-detected from DSN)
- **HTTP testing** — GET/POST/PUT/PATCH/DELETE with headers, JSON body, response assertions
- **Kafka validation** — Produce messages and search/consume with field matching
- **Redis operations** — GET, SET, DEL, EXISTS, KEYS, TTL, HGETALL
- **Shell scripts** — Execute commands with environment injection and exit code assertions
- **Variable chaining** — Pass data between steps with `${var}` and dot notation
- **Expression assertions** — Powered by [expr-lang](https://github.com/expr-lang/expr)
- **Setup/cleanup lifecycle** — Seed databases before, clean up after (always runs)
- **Retry with backoff** — Linear or exponential retry for flaky steps
- **Dry-run mode** — Preview what would execute without running anything
- **JSON output** — Machine-readable results for CI/CD pipelines
- **Profiles** — Switch between environments (dev, staging, prod)

## Quick Start

### Install

**macOS / Linux / Windows (recommended):**

```bash
curl -sSfL https://raw.githubusercontent.com/rakrisi/flowtest/main/install.sh | sh
```

The script auto-detects your OS and architecture, downloads the correct binary from the [latest release](https://github.com/rakrisi/flowtest/releases), and installs it. It falls back to `go install` if the archive is unavailable.

**Go install:**

```bash
go install github.com/rakrisi/flowtest/cmd/flowtest@latest
```

**Build from source:**

```bash
git clone https://github.com/rakrisi/flowtest.git
cd flowtest
go build -o flowtest ./cmd/flowtest/
```

### Initialize

```bash
flowtest init
```

`init` is interactive — it asks which services you use (HTTP, PostgreSQL, MySQL, MongoDB, Redis, Kafka) and generates a minimal config with only what you need. Pass `--no-interactive` to skip prompts (defaults to HTTP only).

This creates:
- `flowtest.yaml` — connection strings for your selected services
- `flows/example.yaml` — a working example flow for those services

### Configure

Edit `flowtest.yaml` with your connection strings:

```yaml
env:
  api_base: http://localhost:8000

  databases:
    db: postgres://user:pass@localhost:5432/myapp
    # mysql_db: mysql://user:pass@localhost:3306/myapp
    # mongo_db: mongodb://localhost:27017/myapp
    # local: sqlite://./test.db

  # kafka_brokers: localhost:9092
  # redis: redis://localhost:6379
```

Database drivers are auto-detected from DSN schemes — no extra configuration needed.

### Write a Flow

```yaml
name: User API Test
description: Verify user CRUD operations

setup:
  - seed:
      table: users
      data: {email: test@example.com, name: Test User}

steps:
  - name: Create user
    api:
      method: POST
      url: /users
      body:
        email: new@example.com
        name: New User
    assert:
      - expr: "response.status == 201"
      - expr: "response.body.id > 0"
    save:
      user_id: response.body.id

  - name: Verify in database
    db:
      query: "SELECT * FROM users WHERE id = $1"
      params: [${user_id}]
    assert:
      - expr: "row_count == 1"
      - expr: "rows[0].email == 'new@example.com'"

  - name: Check Kafka event
    kafka:
      topic: user-events
      match: {user_id: ${user_id}}
      timeout: 5s
    assert:
      - expr: "message.payload.action == 'created'"

  - name: Verify cache
    redis:
      action: get
      key: "user:${user_id}"
    assert:
      - expr: "exists == true"

cleanup:
  - db:
      query: "DELETE FROM users WHERE email = 'new@example.com'"
```

### Run

```bash
# Execute a flow
flowtest run flows/user-test.yaml

# Verbose mode — see requests, queries, assertions
flowtest run flows/user-test.yaml -v

# Dry-run — see plan without executing
flowtest run flows/user-test.yaml --dry-run

# Stop on first failure
flowtest run flows/user-test.yaml --fail-fast

# Inject variables
flowtest run flows/user-test.yaml --var token=abc123

# Use a profile
flowtest run flows/user-test.yaml -p staging

# JSON output for CI
flowtest run flows/user-test.yaml --output-json
```

## CLI Reference

| Command | Description |
|---------|-------------|
| `flowtest run <file>` | Execute a flow |
| `flowtest validate <file>` | Parse and validate without executing |
| `flowtest list [dir]` | List all flow files |
| `flowtest init` | Scaffold a new project |

### Run Flags

| Flag | Description |
|------|-------------|
| `-p, --profile` | Environment profile from flowtest.yaml |
| `-v, --verbose` | Show request/response details, queries, assertions |
| `--fail-fast` | Stop on first failure |
| `--output-json` | Output results as JSON |
| `--html-report FILE` | Generate HTML report (e.g., `--html-report report.html`) |
| `--from-json` | Load flow from JSON file |
| `--var key=value` | Set initial variables (repeatable) |
| `--step N` | Start from step N |
| `--step-name` | Start from the named step |
| `--dry-run` | Preview execution plan |

## HTML Reports

Generate comprehensive HTML reports with Tailwind CSS styling:

```bash
# Single flow with HTML report
flowtest run flows/api-test.yaml --html-report report.html

# Verbose mode + HTML report
flowtest run flows/integration.yaml -v --html-report results.html
```

Reports include:
- ✅ Pass/fail/skip status for each step
- ⏱️ Execution duration and timestamps  
- 📊 Assertions with actual vs expected values
- 🔍 Request/response details (verbose mode)
- 📈 Summary statistics and step breakdown
- 🎨 Beautiful Tailwind CSS design

**Tip:** Open the HTML file in your browser to view the report. Perfect for CI/CD pipelines — commit reports to your repo or upload to artifacts.

## Supported Drivers

### HTTP

```yaml
- name: Create item
  api:
    method: POST
    url: /items
    headers:
      Authorization: "Bearer ${token}"
    body:
      name: Test Item
      price: 29.99
    timeout: 10s
  assert:
    - expr: "response.status == 201"
    - expr: "response.body.name == 'Test Item'"
  save:
    item_id: response.body.id
```

### SQL Databases (PostgreSQL, MySQL, SQLite)

```yaml
# Query
- name: Verify item exists
  db:
    query: "SELECT * FROM items WHERE id = $1"
    params: [${item_id}]
  assert:
    - expr: "row_count == 1"
    - expr: "rows[0].name == 'Test Item'"

# Use a different database
- name: Check MySQL
  mysql_db:
    query: "SELECT COUNT(*) as total FROM orders WHERE user_id = ?"
    params: [${user_id}]
  assert:
    - expr: "rows[0].total > 0"
```

Seed in setup:
```yaml
setup:
  - seed:
      target: db          # optional if only one database
      table: items
      data: {name: Seed Item, price: 10.0}
```

### MongoDB

```yaml
- name: Insert document
  mongo_db:
    collection: users
    operation: insertOne
    document:
      name: Test User
      email: test@example.com
  save:
    doc_id: rows[0]._id

- name: Find documents
  mongo_db:
    collection: users
    operation: find
    filter: {email: test@example.com}
  assert:
    - expr: "row_count >= 1"

- name: Update document
  mongo_db:
    collection: users
    operation: updateOne
    filter: {email: test@example.com}
    update:
      $set: {status: active}
  assert:
    - expr: "modified_count == 1"
```

Supported operations: `find`, `findOne`, `insertOne`, `insertMany`, `updateOne`, `deleteOne`, `deleteMany`, `countDocuments`.

### Kafka

```yaml
# Produce
- name: Send event
  kafka:
    action: produce
    topic: events
    key: order-123
    message: {action: created, order_id: 123}
    headers:
      event-type: order.created

# Consume/search
- name: Find event
  kafka:
    topic: events
    match: {order_id: 123}
    timeout: 10s
  assert:
    - expr: "message.payload.action == 'created'"
```

### Redis

```yaml
- name: Set value
  redis:
    action: set
    key: "session:${user_id}"
    value: {logged_in: true}
    ttl: 1h

- name: Check value
  redis:
    action: get
    key: "session:${user_id}"
  assert:
    - expr: "exists == true"
    - expr: "value.logged_in == true"

- name: Check TTL
  redis:
    action: ttl
    key: "session:${user_id}"
  assert:
    - expr: "expires == true"
    - expr: "ttl > 3500"
```

Actions: `get`, `set`, `del`, `exists`, `keys`, `ttl`, `hgetall`.

### Shell

```yaml
- name: Run script
  script:
    lang: bash
    run: |
      curl -s http://localhost:8000/health | jq -r '.status'
    assert_exit: 0
    timeout: 10s
  assert:
    - expr: "stdout == 'ok'"
```

## Step Features

### Conditional Execution

```yaml
- name: Only if feature enabled
  when: "feature_flag == true"
  api:
    method: GET
    url: /feature-endpoint
```

### Retry

```yaml
- name: Wait for async processing
  retry:
    times: 5
    interval: 2s
    backoff: exponential
  api:
    method: GET
    url: /status/${job_id}
  assert:
    - expr: "response.body.status == 'completed'"
```

### Delay

```yaml
- name: Wait before checking
  delay: 2s
  redis:
    action: get
    key: "result:${job_id}"
```

### Variable Chaining

Steps pass data to subsequent steps via `save`:

```yaml
- name: Create order
  api:
    method: POST
    url: /orders
    body: {item: Widget}
  save:
    order_id: response.body.id

- name: Check order
  api:
    method: GET
    url: /orders/${order_id}
  assert:
    - expr: "response.body.id == order_id"
```

## Multi-Database Configuration

Configure multiple databases by name:

```yaml
# flowtest.yaml
env:
  databases:
    postgres_db: postgres://user:pass@localhost:5432/app
    mysql_db: mysql://user:pass@localhost:3306/app
    mongo_db: mongodb://localhost:27017/app
    local: sqlite://./test.db
```

Reference them in flows by their configured name:

```yaml
steps:
  - name: Query Postgres
    postgres_db:
      query: "SELECT * FROM users"

  - name: Query MySQL
    mysql_db:
      query: "SELECT * FROM orders"

  - name: Query MongoDB
    mongo_db:
      collection: logs
      operation: find
      filter: {level: error}
```

## Profiles

Define environment profiles for different targets:

```yaml
# flowtest.yaml
env:
  api_base: http://localhost:8000
  databases:
    db: postgres://user:pass@localhost:5432/app

profiles:
  staging:
    api_base: https://staging.api.example.com
    databases:
      db: postgres://user:pass@staging-db:5432/app
```

```bash
flowtest run flows/test.yaml -p staging
```

## Try the full demo

The `testproject/` directory is a complete, runnable reference — a Go API server wired to Postgres, MySQL, MongoDB, Redis, and Kafka, with 9 flow files covering every driver.

```bash
cd testproject
./run-tests.sh
```

**Requires Docker.** The script:
1. Builds the FlowTest binary and a test API server
2. Starts all services via Docker Compose (Postgres, MySQL, MongoDB, Redis, Kafka)
3. Validates every flow file
4. Runs all flows with verbose output and generates HTML reports in `test-reports/`

Use it as a copy-paste reference for your own flows.

## Development

```bash
# Run unit tests
go test ./...

# Run with verbose
go test ./... -v

# Build
go build -o flowtest ./cmd/flowtest/

# Vet
go vet ./...
```

## License

MIT
