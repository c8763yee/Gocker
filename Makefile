# Makefile for Gocker
BINARY_NAME=gocker
BUILD_DIR=build
GO_FILES=$(shell find . -name "*.go" -type f)

# Default target
.PHONY: all
all: build

# Build the binary
.PHONY: build
build: $(BUILD_DIR)/$(BINARY_NAME)

$(BUILD_DIR)/$(BINARY_NAME): $(GO_FILES)
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Build completed: $(BUILD_DIR)/$(BINARY_NAME)"

# Clean build artifacts
.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)
	go clean
	@echo "Clean completed"

# Install dependencies
.PHONY: deps
deps:
	go mod download
	go mod tidy
	@echo "Dependencies installed"

# Run tests
.PHONY: test
test:
	go test ./...

# Install the binary to system
.PHONY: install
install: build
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	@echo "Installed to /usr/local/bin/$(BINARY_NAME)"

# Uninstall the binary from system
.PHONY: uninstall
uninstall:
	sudo rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "Uninstalled from /usr/local/bin/$(BINARY_NAME)"

# Setup required directories and permissions
.PHONY: setup
setup:
	sudo mkdir -p /var/lib/gocker/{images,containers}
	sudo chown -R $(USER):$(USER) /var/lib/gocker
	@echo "Setup completed"

# Quick test run
.PHONY: test-run
test-run: build setup
	sudo sysctl -w net.ipv4.ip_forward=1
	sudo $(BUILD_DIR)/$(BINARY_NAME) pull alpine:latest
	sudo $(BUILD_DIR)/$(BINARY_NAME) images
	@echo "Ready to run: sudo $(BUILD_DIR)/$(BINARY_NAME) run alpine:latest /bin/sh"

# Show help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build     - Build the gocker binary"
	@echo "  clean     - Clean build artifacts"
	@echo "  deps      - Install Go dependencies"
	@echo "  test      - Run tests"
	@echo "  install   - Install binary to system"
	@echo "  uninstall - Uninstall binary from system"
	@echo "  setup     - Setup required directories"
	@echo "  test-run  - Quick setup and test"
	@echo "  help      - Show this help message"