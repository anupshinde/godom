.PHONY: build build-examples test test-v cover cover-html vet clean

# Build the library
build:
	go build ./...

# Build examples (compile check only)
# monitor and system-monitor-chartjs have their own go.mod, so they're built separately
build-examples:
	go build ./examples/counter ./examples/clock ./examples/todolist ./examples/todolist-stateful
	cd examples/system-monitor && go build .
	cd examples/system-monitor-chartjs && go build .

# Run all tests
test:
	go test ./...

# Run all tests with verbose output
test-v:
	go test -v ./...

# Run tests with coverage and print summary
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

# Generate HTML coverage report
cover-html: cover
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run go vet
vet:
	go vet ./...

# Run a single test by name: make test-one TEST=TestRender PKG=./
test-one:
	go test -v -run $(TEST) $(PKG)

# Clean generated files
clean:
	rm -f coverage.out coverage.html
