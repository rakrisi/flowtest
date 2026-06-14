# Changelog

All notable changes to FlowTest will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Test tagging support (`tags` field + `--tags` flag for selective runs)

## [1.0.0] - 2026-04-14

### Added
- **Multi-database support** — PostgreSQL, MySQL, MongoDB, SQLite with auto-detection from DSN
- **HTTP driver** — GET, POST, PUT, PATCH, DELETE with JSON body and headers
- **Authentication** — Bearer token, Basic auth, API key (header or query param)
- **Advanced request bodies** — Nested objects, arrays, multi-level nesting with variable interpolation
- **Custom headers** — Arbitrary headers with variable interpolation support
- **Kafka driver** — Produce messages and search/consume with field matching
- **Redis driver** — GET, SET, DEL, EXISTS, KEYS, TTL, HGETALL operations
- **Shell driver** — Execute commands with environment injection and exit code assertions
- **Variable chaining** — Pass data between steps with `${var}` and dot notation
- **Expression assertions** — Powered by [expr-lang](https://github.com/expr-lang/expr)
- **Setup/cleanup lifecycle** — Seed databases before, clean up after (always runs)
- **Conditional execution** — `when` conditions to skip steps based on context
- **Retry with backoff** — Linear or exponential retry for flaky steps
- **Dry-run mode** — Preview what would execute without running anything
- **JSON output** — Machine-readable results for CI/CD pipelines
- **HTML report generation** — Comprehensive reports with Tailwind CSS styling
- **Profiles** — Switch between environments (dev, staging, prod)
- **Flow validation** — Parse and validate flows without executing
- **CLI commands** — `run`, `validate`, `list`, `init`

### CLI Flags
- `-p, --profile` — Environment profile from flowtest.yaml
- `-v, --verbose` — Show request/response details, queries, assertions
- `--fail-fast` — Stop on first failure
- `--output-json` — Output results as JSON
- `--html-report FILE` — Generate HTML report (e.g., `--html-report report.html`)
- `--from-json` — Load flow from JSON file
- `--var key=value` — Set initial variables (repeatable)
- `--step N` — Start from step N
- `--step-name` — Start from the named step
- `--dry-run` — Preview execution plan

## [0.3.0] - 2026-04-13

### Added
- MongoDB driver with find, findOne, insertOne, insertMany, updateOne, deleteOne, deleteMany, countDocuments
- SQLite driver (pure Go, no CGO required)
- MySQL driver with URL-style DSN support
- DSN auto-detection for all database types
- Multi-database flows (cross-database operations in single flow)

### Changed
- Database drivers now use dynamic YAML keys (e.g., `mydb:` instead of fixed `db:`)
- Improved error messages with driver context

## [0.2.0] - 2026-04-10

### Added
- Kafka producer support (`action: produce`)
- Redis HGETALL and TTL operations
- Retry configuration with linear/exponential backoff
- `when` conditions for conditional step execution
- `delay` field for step delays
- Verbose mode with detailed request/response output

### Fixed
- Variable resolution in nested arrays
- Kafka consumer timeout handling

## [0.1.0] - 2026-04-05

### Added
- Initial release
- PostgreSQL driver with query and seed operations
- HTTP driver with basic methods
- Redis driver with basic operations
- Kafka consumer with message matching
- Shell driver for command execution
- YAML flow file format
- Basic CLI with `run` command

[Unreleased]: https://github.com/rakrisi/flowtest/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/rakrisi/flowtest/compare/v0.3.0...v1.0.0
[0.3.0]: https://github.com/rakrisi/flowtest/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/rakrisi/flowtest/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/rakrisi/flowtest/releases/tag/v0.1.0
