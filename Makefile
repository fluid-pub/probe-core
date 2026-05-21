# Makefile for Fluid Agents Core

GO=go
BINARY_NAME=core
BUILD_DIR=build

.PHONY: all build clean test fmt help

all: build

build:
	@echo "Building core CLI..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME) cmd/cli/main.go
	@echo "CLI built in $(BUILD_DIR)/$(BINARY_NAME)"

clean:
	@echo "Cleaning build files..."
	@rm -rf $(BUILD_DIR)

test:
	@echo "Running tests..."
	$(GO) test -v ./...

fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

help:
	@echo "Available commands:"
	@echo "  build         - Build the CLI"
	@echo "  clean         - Clean build files"
	@echo "  test          - Run tests"
	@echo "  fmt           - Format code"
	@echo "  help          - Show this help"
