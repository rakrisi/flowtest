# Contributing to FlowTest

Thank you for your interest in contributing to FlowTest! This document provides guidelines and instructions for contributing.

##  Code of Conduct

By participating in this project, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md). Please read it before contributing.

## How to Contribute

### Reporting Bugs

Before creating a bug report, please check existing issues to avoid duplicates.

**When reporting a bug, include:**

1. **FlowTest version** (`flowtest --version`)
2. **Go version** (`go version`)
3. **Operating system** and version
4. **Flow file** (minimal reproduction case)
5. **Expected behavior** vs **actual behavior**
6. **Error messages** or logs (with `-v` flag)

```bash
# Example bug report command
flowtest run flows/bug-repro.yaml -v 2>&1 | tee bug-output.log
```

### Suggesting Features

Feature requests are welcome! Please include:

1. **Use case** — What problem does this solve?
2. **Proposed solution** — How should it work?
3. **Alternatives considered** — Other approaches you thought of
4. **Flow example** — How would the YAML look?

### Pull Requests

1. **Fork** the repository
2. **Create a branch** from `main`: `git checkout -b feat/my-feature`
3. **Make changes** following our coding standards
4. **Add tests** for new functionality
5. **Run tests**: `go test ./...`
6. **Run linter**: `golangci-lint run`
7. **Commit** with conventional commit format
8. **Push** and create a Pull Request

## Development Setup

### Prerequisites

- Go 1.21 or later
- Docker (for integration tests)
- golangci-lint

### Building

```bash
# Clone
git clone https://github.com/rakrisi/flowtest.git
cd flowtest

# Build
go build -o flowtest ./cmd/flowtest/

# Run tests
go test ./...

# Run linter
golangci-lint run

# Run integration tests (requires Docker)
cd testproject && ./run-tests.sh
```

### Project Structure

```
flowtest/
├── cmd/flowtest/       # CLI entry point
├── internal/
│   ├── config/         # YAML parsing, validation, DSN detection
│   ├── engine/         # Flow execution, variable context, results
│   ├── driver/         # HTTP, DB, Kafka, Redis, Shell drivers
│   └── output/         # Terminal printer, JSON output, reports
├── flows/              # Example flow files
└── testproject/        # Integration test suite
```

### Running Specific Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test ./... -cover

# Run specific package tests
go test ./internal/config/... -v

# Run specific test
go test ./internal/engine/... -run TestContext_ResolveInterface -v
```

## Coding Standards

### Go Style

- Follow [Effective Go](https://go.dev/doc/effective_go)
- Use `gofmt` for formatting
- Pass `golangci-lint` checks
- Write table-driven tests
- Document exported functions and types

### Commit Messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

**Types:**
- `feat` — New feature
- `fix` — Bug fix
- `docs` — Documentation only
- `style` — Formatting, no logic change
- `refactor` — Code restructuring
- `perf` — Performance improvement
- `test` — Adding/fixing tests
- `chore` — Maintenance tasks
- `ci` — CI/CD changes

**Examples:**
```
feat(driver): add gRPC driver support
fix(http): handle timeout correctly for large responses
docs(readme): add installation via Homebrew
test(engine): increase coverage for retry logic
```

### Error Handling

- Wrap errors with context: `fmt.Errorf("driver: operation: %w", err)`
- Never silently ignore errors
- Return errors to the caller; let the engine decide how to handle them

### Adding a New Driver

1. Create `internal/driver/mydriver.go`
2. Implement the `Driver` interface:
   ```go
   type Driver interface {
       Name() string
       Execute(ctx context.Context, stepConfig interface{}, flowCtx *Context, env *config.EnvConfig) (map[string]interface{}, error)
   }
   ```
3. Add config type to `internal/config/schema.go`
4. Register in `internal/driver/registry.go`
5. Add tests in `internal/driver/driver_test.go`
6. Add integration test flow in `testproject/flows/`
7. Document in README.md and AGENTS.md

## Testing

### Unit Tests

- Use table-driven tests for multiple cases
- Mock external dependencies
- Test error paths, not just happy paths

```go
func TestMyFunction(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"valid input", "hello", "HELLO", false},
        {"empty input", "", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := MyFunction(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("got = %q, want %q", got, tt.want)
            }
        })
    }
}
```

### Integration Tests

Integration tests are in `testproject/` and require Docker:

```bash
cd testproject
docker-compose up -d
./run-tests.sh
```

## Release Process

1. Update version in `cmd/flowtest/main.go`
2. Update `CHANGELOG.md`
3. Create PR with title `chore: release vX.Y.Z`
4. After merge, create and push tag:
   ```bash
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0
   ```
5. GoReleaser automatically builds and publishes

## Getting Help

- **GitHub Issues** — Bug reports and feature requests
- **GitHub Discussions** — Questions and community help
- **README.md** — Usage documentation
- **AGENTS.md** — Architecture and internals

## Recognition

Contributors are recognized in:
- GitHub contributors list
- Release notes for significant contributions

Thank you for contributing to FlowTest!
