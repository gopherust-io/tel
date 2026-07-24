export GOWORK := off

PKGS := $(shell go list ./... | grep -vE '/examples/')
NPROCS := $(shell getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)
GO_TEST_FLAGS := -count=1 -parallel=$(NPROCS) -timeout=60s
COVERAGE_MIN ?= 70

.PHONY: help test test-race coverage coverage-html bench ci vet fmt fmt-check lint lint-fix govulncheck examples demo

GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
GOVULNCHECK := go run golang.org/x/vuln/cmd/govulncheck@v1.6.0

help:
	@echo "Targets:"
	@echo "  demo              Run examples/basic"
	@echo "  test              Run all unit tests"
	@echo "  test-race         Run tests with -race"
	@echo "  coverage          Write coverage.out and enforce COVERAGE_MIN ($(COVERAGE_MIN)%)"
	@echo "  coverage-html     Open HTML coverage report"
	@echo "  bench             Run all benchmarks"
	@echo "  ci                fmt-check + unit tests + race + vet + lint"
	@echo "  fmt               gofmt -w"
	@echo "  fmt-check         fail if any file needs gofmt"
	@echo "  lint              govulncheck + golangci-lint"
	@echo "  examples          Build example programs"

demo:
	go run ./examples/basic

test:
	go test $(GO_TEST_FLAGS) $(PKGS)

test-race:
	go test -race $(GO_TEST_FLAGS) $(PKGS)

coverage:
	go test $(GO_TEST_FLAGS) $(PKGS) -coverprofile=coverage.out
	@total=$$(go tool cover -func=coverage.out | awk '/^total:/ {gsub(/%/,"",$$NF); print $$NF}'); \
	echo "Total coverage: $${total}% (minimum $(COVERAGE_MIN)%)"; \
	awk -v t="$${total}" -v m="$(COVERAGE_MIN)" 'BEGIN { if (t+0 < m+0) { print "coverage below threshold"; exit 1 } }'
	go tool cover -func=coverage.out

coverage-html: coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "Wrote coverage.html"

bench:
	go test -bench=. -benchmem $(PKGS) -run '^$$'

ci: fmt-check test test-race vet lint

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

lint: govulncheck
	$(GOLANGCI_LINT) run ./...

lint-fix:
	$(GOLANGCI_LINT) run ./... --fix

govulncheck:
	$(GOVULNCHECK) ./...

examples:
	go build -o /dev/null ./examples/...
