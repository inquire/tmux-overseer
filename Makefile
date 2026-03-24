.PHONY: build install hook install-hook install-all test lint lint-fix vet fmt clean run popup check

BIN_NAME=tmux-overseer
CMD_DIR=./cmd/tmux-overseer

build:
	@echo "Building $(BIN_NAME)..."
	go build -o $(BIN_NAME) $(CMD_DIR)

install:
	@echo "Installing $(BIN_NAME)..."
	go install $(CMD_DIR)

hook:
	@echo "Building claude-hook..."
	go build -trimpath -ldflags '-s -w' -o claude-hook ./cmd/claude-hook

install-hook: hook
	@echo "Installing claude-hook..."
	cp claude-hook $$(go env GOPATH)/bin/claude-hook

install-all: install install-hook

test:
	@echo "Running tests..."
	go test -v ./...

test-race:
	@echo "Running tests with race detector..."
	go test -v -race ./...

lint:
	@echo "Running linter..."
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed. Install with: brew install golangci-lint"; exit 1; }
	golangci-lint run ./...

lint-fix:
	@echo "Running linter with auto-fix..."
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed. Install with: brew install golangci-lint"; exit 1; }
	golangci-lint run --fix ./...

vet:
	@echo "Running go vet..."
	go vet ./...

fmt:
	@echo "Formatting code..."
	gofmt -w .
	@command -v goimports >/dev/null 2>&1 && goimports -w . || true

check: fmt vet lint test
	@echo "All checks passed!"

clean:
	@echo "Cleaning..."
	rm -f $(BIN_NAME) claude-hook

run: build
	@./$(BIN_NAME)

# Quick tmux popup shortcut (run in a popup from any tmux session)
popup: build
	tmux display-popup -E -w 90% -h 90% './$(BIN_NAME)'
