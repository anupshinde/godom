# Testing conventions

How tests are written and reviewed in godom. Read this before adding or modifying tests.

## Ground rules

- **Test behavior, not implementation.** Prefer tests that assert observable outcomes over tests that pin internal details.
- **Cover success *and* failure/edge cases** where they meaningfully exist.
- **Never weaken an assertion to make a test pass.** If production behavior is wrong, let the test fail and report it — don't paper over it.
- **Stop before changing production code.** Keep changes within the target package's `*_test.go` files. If a test exposes a bug whose fix needs non-test changes, get approval first.
- **No `t.Skip` by default.** Only skip when a test truly depends on an unavailable prerequisite (platform, tool, browser, external binary), and state the reason.
- **Helpers and fixtures live in test files**, never in production code.
- **Race-test concurrency.** Anything touching goroutines, channels, or the event loop must pass `go test -race`. Make timing deterministic (poll for a condition with a deadline; don't rely on `time.Sleep` for correctness, and don't assume socket accept-order matches dial-order).

## Coverage

The `cover-check` gate requires **≥ 90%** total across the covered packages (`go test ./... -coverprofile` over the lib + internal packages). But coverage is a floor, not the goal:

- **Aim for meaningful coverage, not a number.** Prefer a few strong tests over many shallow ones. 100% is a natural stretch goal, not a reason to add low-value tests.
- **Prioritize** public behavior, branching logic, boundary conditions, and error handling.
- **Don't pad coverage.** Defensive guards and fault-injection paths (e.g. a marshal failure on a valid message, a send on a broken socket, an unreachable nil check) may be left uncovered — but **document which ones, and why**, in your summary.
- Coverage is per-package: a package's code is only counted by *its own* `*_test.go`. Code exercised only by another package's tests won't show as covered — put unit tests in the package that owns the code.

## Process

1. Read the production code in the target package.
2. Review existing tests for correctness, duplication, and missing coverage.
3. List the important behaviors that are still untested.
4. Add tests for those behaviors, including error and edge cases.
5. Run the gates (below); inspect failures and gaps.
6. Iterate until remaining gaps are low-value, environment-dependent, or otherwise not worth the complexity.
7. Summarize any meaningful coverage gaps and the reason they remain.

## Verification commands

The `make` targets are the canonical gate (see the `Makefile`):

```sh
make test            # go test ./...
make vet             # go vet ./...
make cover-check     # coverage with the 90% floor
make build-examples  # all examples compile
go test -race ./...  # race detector — run for any concurrency change
```

Single package / single test, with coverage detail:

```sh
go test ./internal/<pkg>/ -run TestName -v
go test ./internal/<pkg>/ -coverprofile=/tmp/cover.out
go tool cover -func=/tmp/cover.out          # per-function gaps
go tool cover -html=/tmp/cover.out -o /tmp/coverage.html
```
