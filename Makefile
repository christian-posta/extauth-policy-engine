.PHONY: help build run clean generate deps

# Default target
help:
	@echo "Available targets:"
	@echo "  deps      - Install dependencies and generate protobuf code"
	@echo "  build     - Build the policy engine binary"
	@echo "  run       - Run the policy engine service"
	@echo "  clean     - Clean generated files and binary"
	@echo "  generate  - Generate protobuf code only"

# Install dependencies and generate protobuf code
deps: generate
	go mod tidy

# Generate protobuf code using protoc
generate:
	@echo "Generating protobuf code..."
	mkdir -p gen
	protoc --go_out=gen --go_opt=paths=source_relative \
		--go-grpc_out=gen --go-grpc_opt=paths=source_relative \
		proto/ext_authz.proto

# Build the binary
build: deps
	go build -o policy-engine main.go

# Run the service
run: build
	./policy-engine

# Clean up
clean:
	rm -f policy-engine
	rm -rf gen/

# Check if protoc is installed
check-protoc:
	@which protoc > /dev/null || (echo "Error: protoc is not installed. Please install Protocol Buffers compiler." && exit 1)
