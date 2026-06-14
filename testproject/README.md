# FlowTest Integration Tests

This directory contains comprehensive integration tests for FlowTest.

## Quick Start

```bash
# Run all tests and generate HTML reports
./run-tests.sh
```

HTML reports will be generated in `./test-reports/` directory.

## Requirements

- Docker (for Postgres, MySQL, MongoDB, Redis, Kafka)
- Go 1.21+ (for building test server)
- `nc` (netcat) for health checks
- `curl` for API health checks

## Services

The test suite starts these Docker services:

| Service | Port | Usage |
|---------|------|-------|
| PostgreSQL | 5432 | Primary database tests |
| MySQL | 3306 | MySQL-specific tests |
| MongoDB | 27017 | MongoDB driver tests |
| Redis | 6379 | Cache validation tests |
| Kafka | 9092 | Event stream tests |
| Zookeeper | 2181 | Kafka dependency |

## Test Flows

| Flow | Steps | Tests |
|------|-------|-------|
| 01-health-and-db-seed | 4 | Health endpoint, DB seed/query |
| 02-api-crud | 11 | POST/GET/PUT/PATCH/DELETE |
| 03-redis-validation | 12 | Cache operations |
| 04-kafka-validation | 8 | Event publishing/consuming |
| 05-full-integration | 15 | End-to-end workflow |
| 06-mysql-validation | — | MySQL-specific features |
| 07-mongo-validation | — | MongoDB operations |
| 08-sqlite-validation | — | SQLite driver |
| 09-multi-db-integration | — | Cross-database ops |
| 10-auth-validation | 16 | Bearer/Basic/API key auth |
| 11-advanced-body-response | 9 | Nested objects, arrays |

## HTML Reports

After running tests, HTML reports are available at:

```
./test-reports/
├── 01-health-and-db-seed.html
├── 02-api-crud.html
├── 03-redis-validation.html
├── ...
└── 11-advanced-body-response.html
```

Open any report in your browser:

```bash
# macOS
open test-reports/05-full-integration.html

# Linux
xdg-open test-reports/05-full-integration.html

# Windows
start test-reports/05-full-integration.html
```

Or access via file:// URL shown at end of test run.

## Manual Testing

Run individual flows:

```bash
# Run single flow without HTML report
../flowtest run flows/01-health-and-db-seed.yaml -v

# Run with HTML report
../flowtest run flows/02-api-crud.yaml -v --html-report my-report.html

# Validate flow without executing
../flowtest validate flows/05-full-integration.yaml

# Dry-run (see what would execute)
../flowtest run flows/03-redis-validation.yaml --dry-run
```

## Cleanup

```bash
# Stop all Docker services
docker compose down

# Remove volumes (clean slate)
docker compose down -v

# Remove test reports
rm -rf test-reports/
```

## Troubleshooting

### Services not starting

```bash
# Check Docker is running
docker ps

# View service logs
docker compose logs postgres
docker compose logs kafka

# Restart services
docker compose restart
```

### API server not responding

```bash
# Check if port 8000 is in use
lsof -i :8000

# View test server logs (run-tests.sh shows output)
```

### Flow tests failing

```bash
# Run with verbose output
../flowtest run flows/failing-flow.yaml -v

# Check HTML report for detailed failure info
open test-reports/failing-flow.html

# Check if services are healthy
docker compose ps
```
