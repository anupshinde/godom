.PHONY: build build-examples test test-v cover cover-check cover-html vet clean

# Build the library
build:
	go build ./...

# Build examples (compile check only)
# Some examples have their own go.mod, so they're built separately
build-examples:
	go build ./examples/counter ./examples/clock ./examples/todolist ./examples/charts-without-plugin ./examples/solar-system ./examples/drag-demo ./examples/drag-tiles ./examples/progress-bar ./examples/stock-ticker ./examples/sync-demo ./examples/basic-form-builder ./examples/video-player ./examples/breakout-game ./examples/multi-component ./examples/select-test
	cd examples/system-monitor && go build .
	cd examples/system-monitor-chartjs && go build .
	cd examples/markdown-editor && go build .
	cd examples/terminal && go build .

# Validate all examples (Mount + directive validation, no server)
validate-examples:
	@for d in examples/counter examples/clock examples/todolist examples/charts-without-plugin examples/solar-system examples/drag-demo examples/drag-tiles examples/progress-bar examples/stock-ticker examples/sync-demo examples/basic-form-builder examples/breakout-game examples/multi-component examples/select-test; do \
		printf "%-25s " "$$(basename $$d)"; \
		GODOM_VALIDATE_ONLY=1 go run ./$$d 2>&1 && echo "OK" || echo "FAIL"; \
	done
	@printf "%-25s " "video-player"; \
	GODOM_VALIDATE_ONLY=1 go run ./examples/video-player -video /dev/null 2>&1 && echo "OK" || echo "FAIL"
	@for d in examples/system-monitor examples/system-monitor-chartjs examples/markdown-editor examples/terminal; do \
		printf "%-25s " "$$(basename $$d)"; \
		cd $$d && GODOM_VALIDATE_ONLY=1 go run . 2>&1 && echo "OK" || echo "FAIL"; \
		cd ../..; \
	done

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
