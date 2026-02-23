# Grimoire CLI

A local-first CLI tool written in Go that provides a shared file-based knowledge store for AI agents.

## Project Structure

```
cmd/grimoire-cli/      Entry point (main.go)
internal/              All internal packages (see design/low-level-design.md)
templates/             Embedded templates for doc generation and platform skills
hooks/                 Git hook scripts (source of truth)
design/                Requirements, high-level, and low-level design docs
plan/                  Implementation plan and task tracking
reviews/               Design review documents
```

## Code Quality & Development Commands

### Quick Reference

| Action | Command |
|--------|---------|
| Build binary | `make build` |
| Install binary | `make install` |
| Run tests | `make test` |
| Run tests with coverage | `make test-coverage` |
| Run linter | `make lint` |
| Auto-format | `make fmt` |
| Check formatting | `make fmt-check` |
| Run go vet | `make vet` |
| Run dependency audit | `make audit` |
| Run all checks | `make check` |
| Install pre-commit hooks | `make setup-hooks` |
| Clean build artifacts | `make clean` |

### Coverage Requirements

- **Line coverage:** 95%
- Coverage is enforced by pre-commit hooks and `make test-coverage`.
- Tests that reduce coverage below the threshold will block commits.

### Pre-Commit Hooks

The following checks run automatically on `git commit`:

1. **Auto-format** — `gofmt` + `goimports` (fixes and re-stages)
2. **Lint** — `golangci-lint run ./...` (fails on errors)
3. **Vet** — `go vet ./...` (fails on issues)
4. **Test with coverage** — fails if below 95% threshold
5. **Dependency audit** — `govulncheck` (warns on vulnerabilities, non-blocking)

To install hooks: `make setup-hooks`
To bypass hooks (emergency only): `git commit --no-verify`

### Before Submitting Code

Always run the full check suite:

```
make check
```

If coverage is below threshold, add tests for uncovered code paths before committing.

### Tool Requirements

These must be installed and on `$PATH`:

- **Go 1.22+** — `go version`
- **golangci-lint** — `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`
- **goimports** — `go install golang.org/x/tools/cmd/goimports@latest`
- **govulncheck** — `go install golang.org/x/vuln/cmd/govulncheck@latest`

### Key Technical Decisions

- Pure-Go SQLite via `modernc.org/sqlite` (no CGO)
- Cobra CLI framework with Viper configuration
- Constructor injection; composition root in `main.go`
- Repository interfaces for data access
- `internal/` packages — all packages are internal to the module
- Test with real SQLite in temp directories (no mocks for data access)
