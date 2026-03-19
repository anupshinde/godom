.PHONY: build build-examples test test-v cover cover-check cover-html vet clean

# Build the library
build:
	go build ./...

# Build examples (compile check only)
# monitor and system-monitor-chartjs have their own go.mod, so they're built separately
build-examples:
	go build ./examples/counter ./examples/clock ./examples/todolist ./examples/todolist-stateful ./examples/charts-without-plugin ./examples/solar-system
	cd examples/system-monitor && go build .
	cd examples/system-monitor-chartjs && go build .

# Run all tests
test:
	go test ./...

# Run all tests with verbose output
test-v:
	go test -v ./...

COVERAGE_MIN := 90
COVERAGE_PKGS := ./ ./internal/component ./internal/render ./internal/server ./internal/template ./internal/vdom ./plugins/...

# Run tests with coverage and print summary
cover:
	go test -coverprofile=coverage.out $(COVERAGE_PKGS)
	go tool cover -func=coverage.out

# Run tests with coverage and fail if below threshold
cover-check:
	go test -coverprofile=coverage.out $(COVERAGE_PKGS)
	@total=$$(go tool cover -func=coverage.out | grep '^total:' | awk '{print $$NF}' | tr -d '%'); \
	echo "Total coverage: $${total}% (minimum: $(COVERAGE_MIN)%)"; \
	if [ $$(echo "$${total} < $(COVERAGE_MIN)" | bc) -eq 1 ]; then \
		echo "FAIL: coverage $${total}% is below $(COVERAGE_MIN)%"; \
		exit 1; \
	fi

# Generate HTML coverage report and open in browser
cover-html:
	go test -coverprofile=coverage.out $(COVERAGE_PKGS)
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"
	open coverage.html

# Run go vet
vet:
	go vet ./...

# Run a single test by name: make test-one TEST=TestRender PKG=./
test-one:
	go test -v -run $(TEST) $(PKG)

# Clean generated files
clean:
	rm -f coverage.out coverage.html
