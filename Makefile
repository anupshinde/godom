.PHONY: build build-examples test test-v cover cover-check cover-html vet clean

# --- Example lists ---
# Main-module examples (share the root go.mod)
MAIN_EXAMPLES := \
	basic-form-builder \
	breakout-game \
	chart-plugins \
	charts-without-plugin \
	clock \
	counter \
	crash-test \
	drag-demo \
	drag-tiles \
	dynamic-mount \
	embedded-widget \
	exec-and-call \
	multi-island \
	multi-page \
	multi-page-v2 \
	progress-bar \
	same-island-repeated \
	select-test \
	shared-state \
	solar-system \
	stock-ticker \
	sync-demo \
	todolist \
	video-player \
	ws-lifecycle

# Sub-module examples (have their own go.mod — must cd into each)
SUBMODULE_EXAMPLES := \
	markdown-editor \
	system-monitor \
	system-monitor-chartjs \
	terminal

# Build the library
build:
	go build ./...

# Build examples (compile check only)
build-examples:
	@for d in $(MAIN_EXAMPLES); do \
		printf "%-25s " "$$d"; \
		go build -o /dev/null ./examples/$$d && echo "OK" || exit 1; \
	done
	@for d in $(SUBMODULE_EXAMPLES); do \
		printf "%-25s " "$$d"; \
		(cd examples/$$d && go build -o /dev/null .) && echo "OK" || exit 1; \
	done

# Validate all examples (Register + directive validation, no server)
validate-examples:
	@for d in $(MAIN_EXAMPLES); do \
		printf "%-25s " "$$d"; \
		if [ "$$d" = "video-player" ]; then \
			GODOM_VALIDATE_ONLY=1 GODOM_NO_BROWSER=1 go run ./examples/$$d -video /dev/null >/dev/null 2>&1 && echo "OK" || { echo "FAIL"; exit 1; }; \
		else \
			GODOM_VALIDATE_ONLY=1 GODOM_NO_BROWSER=1 go run ./examples/$$d >/dev/null 2>&1 && echo "OK" || { echo "FAIL"; exit 1; }; \
		fi; \
	done
	@for d in $(SUBMODULE_EXAMPLES); do \
		printf "%-25s " "$$d"; \
		(cd examples/$$d && GODOM_VALIDATE_ONLY=1 GODOM_NO_BROWSER=1 go run . >/dev/null 2>&1) && echo "OK" || { echo "FAIL"; exit 1; }; \
	done

# Run all tests
test:
	go test ./...

# Run all tests with verbose output
test-v:
	go test -v ./...

COVERAGE_MIN := 90
COVERAGE_PKGS := ./ ./internal/island ./internal/render ./internal/server ./internal/template ./internal/vdom ./plugins/...

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
