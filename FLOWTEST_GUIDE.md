# FlowTest YAML Authoring Guide

For AI agents and developers writing flow files from scratch. Read this before generating any YAML.

---

## Prerequisites

### Install FlowTest

```bash
# Build from source
git clone https://github.com/radhe-singh/flowtest
cd flowtest
go build -o flowtest ./cmd/flowtest/
```

### Scaffold a new project

```bash
./flowtest init
# Creates: flowtest.yaml  flows/example.yaml
```

### Validate before running

```bash
./flowtest validate flows/my-flow.yaml          # parse only
./flowtest validate flows/my-flow.yaml --check  # parse + TCP reachability check
./flowtest run flows/my-flow.yaml --dry-run      # show plan, skip execution
./flowtest run flows/my-flow.yaml -v             # execute with full output
```

---

## Project Layout

```
my-project/
├── flowtest.yaml       # global config — connection strings and profiles
└── flows/
    ├── 01-seed.yaml
    ├── 02-crud.yaml
    └── 03-events.yaml
```

Run all flows in order:

```bash
for f in flows/*.yaml; do ./flowtest run "$f" -v || exit 1; done
```

---

## Global Config (`flowtest.yaml`)

Every field here is optional and can be overridden per flow.

```yaml
env:
  api_base: http://localhost:8000

  databases:
    db:       postgres://user:pass@localhost:5432/mydb?sslmode=disable
    mysql_db: mysql://user:pass@localhost:3306/mydb
    mongo_db: mongodb://localhost:27017/mydb
    local:    sqlite://./test.db

  kafka_brokers: localhost:9092
  redis: redis://localhost:6379

profiles:
  staging:
    api_base: https://staging.api.example.com
    databases:
      db: postgres://user:pass@staging-db:5432/mydb
  production:
    api_base: https://api.example.com
```

Switch profile at runtime: `./flowtest run flow.yaml -p staging`

### DSN auto-detection rules

| Prefix | Driver |
|--------|--------|
| `postgres://` or `postgresql://` | PostgreSQL |
| `mysql://` | MySQL |
| `mongodb://` or `mongodb+srv://` | MongoDB |
| `sqlite://` or `.db`/`.sqlite` extension | SQLite |

---

## Flow File Structure

```yaml
name: My Flow                  # REQUIRED
description: What this tests   # optional
timeout: 60s                   # optional — global timeout for the whole flow
fail_fast: true                # optional — stop on first failure

env:                           # optional — overrides global config for this flow only
  api_base: http://localhost:8000
  databases:
    db: postgres://user:pass@localhost:5432/mydb

setup:   # optional — runs once before steps (seed data, pre-conditions)
  - ...

cleanup: # optional — ALWAYS runs after steps, even on failure
  - ...

steps:   # REQUIRED — at least one step
  - ...
```

---

## Steps

Every step has:

```yaml
- name: Descriptive Step Name       # REQUIRED
  when: "condition_expr"            # optional — skip step if evaluates to false
  delay: 500ms                      # optional — pause before this step
  retry:                            # optional — retry on assertion failure
    times: 3
    interval: 1s
    backoff: exponential            # "linear" (default) or "exponential"

  # Exactly ONE of: api, <db_key>, kafka, redis, script
  api: ...

  assert:                           # optional — fail step if any expr is false
    - expr: "..."
      msg: "optional failure message"

  save:                             # optional — store values into flow context
    var_name: path.to.value         # only runs if ALL assertions pass
```

---

## Drivers

### HTTP (`api`)

```yaml
- name: Create User
  api:
    method: POST        # GET POST PUT PATCH DELETE
    url: /users         # relative → prepends env.api_base; absolute → used as-is
    headers:
      X-Request-ID: req-001
      X-Trace-ID: trace-${order_id}
    auth:
      bearer: ${token}          # OR:
      # basic:
      #   username: admin
      #   password: ${password}
      # api_key:
      #   header: X-API-Key     # OR query: api_key
      #   value: ${api_key}
    body:
      email: user@example.com
      nested:
        role: admin
      items:
        - id: 1
          name: Widget
      tags: [urgent, priority]
    timeout: 10s
  assert:
    - expr: "response.status == 201"
    - expr: "response.body.id > 0"
  save:
    user_id: response.body.id
    token:   response.body.token
```

**HTTP driver result keys:**

| Key | Type | Description |
|-----|------|-------------|
| `response.status` | int | HTTP status code |
| `response.headers` | map | Response headers (lowercase keys) |
| `response.body` | any | Parsed JSON body or raw string |

**Rules:**
- Setting `body` auto-adds `Content-Type: application/json`. Do not add it manually.
- Only one `auth` type per step (bearer, basic, or api_key).
- `auth` and `headers` can coexist.
- Relative URLs require `env.api_base` to be set.

---

### SQL Databases (`db`, `mysql_db`, `local`, or any name from `databases`)

The key name in the step MUST match the name defined under `databases` in the config.

**Query:**

```yaml
- name: Get User
  db:                                        # key matches databases.db
    query: "SELECT id, email FROM users WHERE id = $1"
    params:
      - ${user_id}                           # ${var} resolved before execution
  assert:
    - expr: "row_count == 1"
    - expr: "rows[0].email == 'user@example.com'"
  save:
    user_email: rows[0].email
```

**MySQL / SQLite** — use `?` placeholders, not `$1`:

```yaml
- name: MySQL Query
  mysql_db:
    query: "SELECT id, name FROM items WHERE status = ?"
    params:
      - active
```

**SQLite:**

```yaml
- name: SQLite Query
  local:
    query: "SELECT * FROM products WHERE id = ?"
    params:
      - ${product_id}
```

**SQL driver result keys:**

| Key | Type | Description |
|-----|------|-------------|
| `rows` | `[]map[string]any` | All returned rows |
| `row_count` | int | Number of rows |
| `rows[0].column_name` | any | First row, specific column |

**Seed (setup/cleanup only):**

```yaml
setup:
  - seed:
      target: db            # optional if only one database; REQUIRED if multiple
      table: users
      data:
        id: 42
        email: seed@test.com
        role: admin
```

Seed uses `INSERT ... ON CONFLICT DO NOTHING` (Postgres) or `INSERT IGNORE` (MySQL) or `INSERT OR IGNORE` (SQLite).

---

### MongoDB (`mongo_db` or any name from `databases` with a `mongodb://` DSN)

```yaml
- name: Insert Document
  mongo_db:
    collection: orders
    operation: insertOne
    document:
      name: "Widget"
      price: 9.99
      source: flowtest
  assert:
    - expr: "row_count == 1"
  save:
    doc_id: rows[0]._id
```

**Operations:**

| Operation | Required fields | Returns |
|-----------|----------------|---------|
| `find` | `filter` | `rows` (all docs), `row_count` |
| `findOne` | `filter` | `rows` (0 or 1), `row_count` |
| `insertOne` | `document` | `rows` ([{_id}]), `row_count` = 1 |
| `insertMany` | `documents` | `rows` ([{_id}...]), `row_count` |
| `updateOne` | `filter`, `update` | `row_count` (matched), `modified_count` |
| `deleteOne` | `filter` | `row_count` (deleted) |
| `deleteMany` | `filter` | `row_count` (deleted) |
| `countDocuments` | `filter` | `row_count` |

**Update syntax** — plain fields are auto-wrapped in `$set`:

```yaml
update:
  price: 44.99        # becomes {$set: {price: 44.99}} automatically
  status: updated

# OR use explicit operators:
update:
  $set:
    price: 44.99
  $unset:
    old_field: ""
```

**Cleanup pattern** — always tag test data so you can delete it reliably:

```yaml
document:
  name: "Test Widget"
  source: flowtest     # tag for cleanup

cleanup:
  - mongo_db:
      collection: orders
      operation: deleteMany
      filter:
        source: flowtest
```

---

### Kafka (`kafka`)

```yaml
# Consume — find a message matching a filter
- name: Verify Order Created Event
  kafka:
    topic: order.created
    timeout: 10s             # how long to search before failing
    match:
      order_id: ${order_id}  # match against JSON payload fields
  assert:
    - expr: "message.payload.status == 'created'"
    - expr: "message.payload.user_id == user_id"
  save:
    event_ts: message.timestamp

# Produce — publish a message
- name: Publish Event
  kafka:
    action: produce
    topic: order.events
    key: "order-${order_id}"
    message:
      order_id: ${order_id}
      status: shipped
    headers:
      event-type: order.shipped
```

**Kafka consume result keys:**

| Key | Type | Description |
|-----|------|-------------|
| `message.topic` | string | Topic name |
| `message.partition` | int | Partition number |
| `message.offset` | int64 | Message offset |
| `message.key` | string | Message key |
| `message.timestamp` | int64 | Unix timestamp |
| `message.payload` | any | Parsed JSON or raw string |
| `message.headers` | map | Message headers |

**Kafka produce result keys:** `produced.topic`, `produced.key`

**Rules:**
- Kafka reads from `FirstOffset` on ALL partitions every time — there are no consumer groups.
- ALWAYS use `match` to identify the correct message, especially on busy topics.
- Without `match`, the step returns the first message on the topic, which may be stale.
- If no message matches within `timeout`, the step errors.

---

### Redis (`redis`)

```yaml
# GET — read a cached value
- name: Verify Cache
  redis:
    action: get
    key: "item:${item_id}"
  assert:
    - expr: "exists == true"
    - expr: "value.status == 'active'"

# SET — write a value
- name: Set Cache
  redis:
    action: set
    key: "session:${user_id}"
    value: ${session_data}
    ttl: 1h

# EXISTS — check presence without reading
- name: Cache Cleared
  redis:
    action: exists
    key: "item:${item_id}"
  assert:
    - expr: "exists == false"

# TTL — check expiry
- name: Check TTL
  redis:
    action: ttl
    key: "item:${item_id}"
  assert:
    - expr: "expires == true"
    - expr: "ttl > 0"

# KEYS — list matching keys
- name: Count Cached Items
  redis:
    action: keys
    key: "item:*"
  assert:
    - expr: "key_count > 0"    # NOTE: key_count, NOT count

# DEL — delete a key
- name: Evict Cache
  redis:
    action: del
    key: "item:${item_id}"
  assert:
    - expr: "deleted == 1"

# HGETALL — read a hash
- name: Get Hash
  redis:
    action: hgetall
    key: "user:${user_id}:profile"
  assert:
    - expr: "exists == true"
    - expr: "value.email == 'user@example.com'"
```

**Redis result keys by action:**

| Action | Result keys |
|--------|-------------|
| `get` | `value` (parsed JSON or string), `exists` (bool), `raw` (string) |
| `set` | `ok` (bool) |
| `exists` | `exists` (bool), `value` (bool) |
| `hgetall` | `value` (map), `exists` (bool) |
| `keys` | `value` ([]string), `key_count` (int) |
| `del` | `deleted` (int) |
| `ttl` | `ttl` (float64 seconds), `expires` (bool) |

---

### Shell (`script`)

```yaml
- name: Run Script
  script:
    lang: bash           # "bash" or "sh" (default)
    run: |
      echo "item_id=${item_id}"
      curl -sf http://localhost:8000/healthz
    assert_exit: 0
    timeout: 30s
  save:
    script_output: stdout

# Single-line
- name: Print Summary
  script:
    run: echo "Passed — item_id=${item_id}"
    assert_exit: 0
```

**Shell result keys:** `stdout` (string), `stderr` (string), `exit_code` (int)

Flow context variables are injected as env vars: `${item_id}` → `FLOWTEST_ITEM_ID`.

---

## Variables

### Saving values

```yaml
save:
  token:      response.body.token           # HTTP response field
  user_id:    response.body.user.id         # nested path
  first_item: rows[0].name                  # DB result
  item_id:    rows[0]._id                   # MongoDB _id
  count:      row_count                     # scalar
  cached:     value.status                  # Redis value field
  output:     stdout                        # shell stdout
```

`save` only runs when ALL assertions pass.

### Using saved values

```yaml
url: /items/${item_id}                      # in URL
params: [${user_id}]                        # in SQL params
match:
  order_id: ${order_id}                     # in Kafka match filter
key: "item:${item_id}"                      # in Redis key
body:
  user_id: ${user_id}                       # in HTTP body
  nested:
    city: ${customer_city}
headers:
  X-User: ${user_id}                        # in HTTP headers
auth:
  bearer: ${token}                          # in auth
filter:
  email: "${user_email}"                    # in MongoDB filter
```

### Variable scope rules

| Pattern | Resolved at | Source |
|---------|-------------|--------|
| `${UPPER_CASE}` | YAML load time | OS environment variables |
| `${lower_case}` | Step execution time | Flow context (saved from prior steps) |

### Injecting variables from the CLI

```bash
./flowtest run flow.yaml --var user_id=42 --var token=abc123
```

### Conditional steps

```yaml
- name: Only on large orders
  when: "order_id > 1000"
  ...

- name: Only if cache exists
  when: "exists == true"
  ...
```

---

## Assertions

Assertions use [expr-lang](https://github.com/expr-lang/expr). The expression is evaluated against the full flow context (all saved variables + the current step's driver results merged in).

```yaml
assert:
  - expr: "response.status == 201"
  - expr: "response.body.id > 0"
    msg: "ID must be positive"
  - expr: "response.body.token != ''"
    msg: "Token must not be empty"
  - expr: "rows[0].role == 'admin'"
  - expr: "row_count >= 1"
  - expr: "message.payload.status == 'created'"
  - expr: "exists == true"
  - expr: "key_count > 0"             # Redis KEYS
  - expr: "modified_count == 1"       # MongoDB updateOne
  - expr: "len(response.body.items) == 3"   # array length
  - expr: "response.body.body.tags[0] == 'urgent'"
```

**Comparison reference:**

| Goal | Expression |
|------|-----------|
| Exact string match | `response.body.status == 'active'` |
| Non-empty string | `response.body.token != ''` |
| Numeric comparison | `response.body.price > 0` |
| Boolean true | `exists == true` or just `exists` |
| Null/nil check | `response.body.field == nil` |
| Array length | `len(response.body.items) == 3` |
| Array element | `response.body.items[0].id == 1` |
| Cross-step equality | `message.payload.user_id == user_id` |

**JSON numbers unmarshal as float64.** `response.body.id == 5` works because expr-lang normalizes numeric comparisons. But `response.body.id == "5"` (string) will fail.

---

## Retry

Use retry for steps that depend on async processes (Kafka consumers, cache population, eventual consistency):

```yaml
- name: Wait for Cache
  redis:
    action: get
    key: "item:${item_id}"
  retry:
    times: 5
    interval: 500ms
    backoff: exponential      # waits: 500ms, 1s, 2s, 4s, 8s
  assert:
    - expr: "exists == true"
```

Kafka steps have a built-in `timeout` field — use that instead of retry for Kafka.

---

## Setup and Cleanup

```yaml
setup:
  - seed:                              # Insert rows (ON CONFLICT DO NOTHING)
      target: db                       # required when multiple databases configured
      table: users
      data:
        email: test@example.com
        name: Test User
        role: admin

  - db:                                # Raw SQL in setup
      query: "DELETE FROM temp_data WHERE created_at < NOW() - INTERVAL '1 hour'"

  - mongo_db:                          # MongoDB pre-cleanup
      collection: test_items
      operation: deleteMany
      filter:
        source: flowtest

cleanup:
  - db:
      query: "DELETE FROM items WHERE name LIKE 'Test%'"
  - db:
      query: "DELETE FROM users WHERE email = 'test@example.com'"
```

**Cleanup always runs** — even when steps fail or `fail_fast` stops execution early. Put all teardown in `cleanup`, not in the last step.

---

## Multi-Database Flows

When multiple databases are configured, each step references one database by its key name:

```yaml
env:
  databases:
    db:       postgres://...
    mysql_db: mysql://...
    mongo_db: mongodb://...
    local:    sqlite://./test.db

steps:
  - name: Postgres query
    db:
      query: "SELECT id FROM users LIMIT 1"

  - name: MySQL query
    mysql_db:
      query: "SELECT id FROM products WHERE id = ?"
      params: [${item_id}]

  - name: MongoDB query
    mongo_db:
      collection: events
      operation: find
      filter:
        source: flowtest

  - name: SQLite query
    local:
      query: "SELECT * FROM config WHERE key = ?"
      params: [feature_flag]
```

---

## Common Patterns

### Login → authenticated requests

```yaml
steps:
  - name: Login
    api:
      method: POST
      url: /auth/login
      body:
        email: user@example.com
        password: secret
    assert:
      - expr: "response.status == 200"
      - expr: "response.body.token != ''"
    save:
      token: response.body.token

  - name: Authenticated Request
    api:
      method: GET
      url: /me
      auth:
        bearer: ${token}
    assert:
      - expr: "response.status == 200"
```

### Create → verify in DB

```yaml
  - name: Create Item
    api:
      method: POST
      url: /items
      body:
        name: Widget
        price: 9.99
    assert:
      - expr: "response.status == 201"
    save:
      item_id: response.body.id

  - name: Verify in DB
    db:
      query: "SELECT name, price FROM items WHERE id = $1"
      params: [${item_id}]
    assert:
      - expr: "row_count == 1"
      - expr: "rows[0].name == 'Widget'"
      - expr: "rows[0].price == 9.99"
```

### Create → verify Kafka event → verify Redis cache

```yaml
  - name: Create triggers event and cache
    api:
      method: POST
      url: /items
      body:
        name: Widget
    assert:
      - expr: "response.status == 201"
    save:
      item_id: response.body.id

  - name: Kafka event received
    kafka:
      topic: item.created
      timeout: 10s
      match:
        id: ${item_id}
    assert:
      - expr: "message.payload.name == 'Widget'"

  - name: Item is cached after GET
    api:
      method: GET
      url: /items/${item_id}

  - name: Verify Redis cache
    redis:
      action: get
      key: "item:${item_id}"
    assert:
      - expr: "exists == true"
      - expr: "value.name == 'Widget'"
```

### Delete → verify gone everywhere

```yaml
  - name: Delete Item
    api:
      method: DELETE
      url: /items/${item_id}
    assert:
      - expr: "response.status == 200"

  - name: Kafka deleted event
    kafka:
      topic: item.deleted
      timeout: 10s
      match:
        id: ${item_id}

  - name: Cache cleared
    redis:
      action: exists
      key: "item:${item_id}"
    assert:
      - expr: "exists == false"

  - name: DB row gone
    db:
      query: "SELECT id FROM items WHERE id = $1"
      params: [${item_id}]
    assert:
      - expr: "row_count == 0"
```

### Test error paths

```yaml
  - name: 404 on unknown item
    api:
      method: GET
      url: /items/999999
    assert:
      - expr: "response.status == 404"
      - expr: "response.body.error != ''"

  - name: 401 without token
    api:
      method: GET
      url: /me
    assert:
      - expr: "response.status == 401"

  - name: 422 on invalid body
    api:
      method: POST
      url: /items
      body:
        price: -1
    assert:
      - expr: "response.status == 422"
```

---

## Known Gotchas

**1. `count` is reserved in expr-lang.**
Redis `keys` action returns `key_count`, not `count`. Never name a saved variable `count`.

```yaml
# WRONG
assert:
  - expr: "count > 0"        # expr-lang treats count as built-in function

# CORRECT (Redis KEYS)
assert:
  - expr: "key_count > 0"
```

**2. Database key = YAML key name.**
The step key name (e.g. `db:`, `mysql_db:`) must exactly match the name under `databases:` in config. A mismatch causes a "no driver registered" error.

```yaml
# flowtest.yaml
env:
  databases:
    my_pg: postgres://...

# flow.yaml
steps:
  - name: Query
    my_pg:              # must match — not "db:" or "postgres:"
      query: "SELECT 1"
```

**3. Kafka reads from the beginning every time.**
Use `match` on every Kafka consume step to find the right message. Without it you get the oldest message on the topic, not the one you just produced.

**4. `save` is skipped when assertions fail.**
If step N fails its assertions, any variable it was supposed to save is not written. Steps N+1 onward that rely on those variables will fail with unresolved `${var}`. Use `fail_fast: true` to surface this clearly.

**5. PostgreSQL uses `$1 $2`, MySQL/SQLite use `?`.**
Mixing them causes a SQL error at runtime.

**6. Seed `target` is required with multiple databases.**
```yaml
setup:
  - seed:
      target: db        # required if databases map has more than one entry
      table: users
      data: ...
```

**7. `${UPPER_CASE}` from OS env is resolved at load time.**
This means it's baked in before `--profile` is applied. Use `${lower_case}` for values that should come from the flow context or be injected via `--var`.

**8. JSON numbers are float64 in assertions.**
`response.body.count == 3` works. `response.body.id == "3"` (string comparison) will fail.

**9. MongoDB `update` without operators is auto-wrapped in `$set`.**
If you pass `{price: 50}` it becomes `{$set: {price: 50}}`. If you need `$inc`, `$push`, etc., use explicit operators.

**10. Body implies `Content-Type: application/json`.**
Do not add `Content-Type` manually to `headers` when `body` is set. The driver adds it automatically and duplicates cause issues with some servers.

---

## CLI Reference

```bash
# Run
./flowtest run flows/my-flow.yaml
./flowtest run flows/my-flow.yaml -v                         # verbose
./flowtest run flows/my-flow.yaml --fail-fast
./flowtest run flows/my-flow.yaml -p staging                 # use profile
./flowtest run flows/my-flow.yaml --var token=abc            # inject variable
./flowtest run flows/my-flow.yaml --dry-run                  # plan without executing
./flowtest run flows/my-flow.yaml --output-json              # JSON output
./flowtest run flows/my-flow.yaml --step 3                   # start from step 3
./flowtest run flows/my-flow.yaml --step-name "Create Item"  # start from named step

# Validate (no execution)
./flowtest validate flows/my-flow.yaml
./flowtest validate flows/my-flow.yaml --check    # also check TCP connectivity

# List flows
./flowtest list           # lists flows/ directory
./flowtest list ./tests   # lists given directory

# Scaffold
./flowtest init
```

Exit code: `0` if all steps passed or skipped, `1` if any failed or errored.

---

## Minimal Working Examples

### Simplest possible flow

```yaml
name: Health Check
steps:
  - name: API is up
    api:
      method: GET
      url: http://localhost:8000/healthz
    assert:
      - expr: "response.status == 200"
```

### Database seed and query

```yaml
name: DB Seed Test
env:
  databases:
    db: postgres://user:pass@localhost:5432/mydb?sslmode=disable

setup:
  - seed:
      table: users
      data:
        email: seed@test.com
        role: admin

cleanup:
  - db:
      query: "DELETE FROM users WHERE email = 'seed@test.com'"

steps:
  - name: User exists in DB
    db:
      query: "SELECT role FROM users WHERE email = $1"
      params: [seed@test.com]
    assert:
      - expr: "row_count == 1"
      - expr: "rows[0].role == 'admin'"
```

### Full integration checklist per endpoint

For any new endpoint that touches multiple systems, write one flow with this step order:

1. **Setup** — seed required records
2. **API call** — POST/PUT/PATCH/DELETE
3. **Assert response** — status, body fields
4. **DB verify** — row state matches
5. **Kafka verify** — event published with correct payload
6. **Redis verify** — cache populated or cleared
7. **Cleanup** — delete test data

---

## Integration with Docker Compose

Standard service config for FlowTest:

```yaml
# docker-compose.yaml
services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_USER: myuser
      POSTGRES_PASSWORD: mypassword
      POSTGRES_DB: mydb
    ports: ["5432:5432"]

  mysql:
    image: mysql:8
    environment:
      MYSQL_USER: myuser
      MYSQL_PASSWORD: mypassword
      MYSQL_DATABASE: mydb
      MYSQL_ROOT_PASSWORD: root
    ports: ["3306:3306"]

  mongo:
    image: mongo:7
    ports: ["27017:27017"]

  redis:
    image: redis:7
    ports: ["6379:6379"]

  kafka:
    image: confluentinc/cp-kafka:7.5.0
    environment:
      KAFKA_ZOOKEEPER_CONNECT: zookeeper:2181
      KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://localhost:9092
    ports: ["9092:9092"]
    depends_on: [zookeeper]

  zookeeper:
    image: confluentinc/cp-zookeeper:7.5.0
    environment:
      ZOOKEEPER_CLIENT_PORT: 2181
```

Matching `flowtest.yaml`:

```yaml
env:
  api_base: http://localhost:8000
  kafka_brokers: localhost:9092
  redis: redis://localhost:6379
  databases:
    db:       postgres://myuser:mypassword@localhost:5432/mydb?sslmode=disable
    mysql_db: mysql://myuser:mypassword@localhost:3306/mydb
    mongo_db: mongodb://localhost:27017/mydb
    local:    sqlite://./test.db
```
