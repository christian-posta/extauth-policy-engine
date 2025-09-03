# Policy Engine - External Authorization Service

This is a sample external authorization service that implements the Envoy ext_authz protocol. It's designed to work with AgentGateway to demonstrate external policy engine integration.

## Features

The policy engine implements several simple policies to demonstrate the full context that AgentGateway sends:

1. **Path-based restrictions**: Denies access to `/admin/*` paths
2. **Method-based restrictions**: Requires `Authorization` header for POST requests
3. **Time-based restrictions**: Only allows access during business hours (9 AM - 5 PM)
4. **Environment-based restrictions**: Uses context extensions from AgentGateway config
5. **Header-based restrictions**: Blocks bot user agents
6. **Header manipulation**: Adds authorization metadata and removes sensitive headers

## What Context Does AgentGateway Send?

The service logs all the context it receives from AgentGateway, including:

- **HTTP Request**: method, path, host, scheme, protocol, headers, body size
- **Timing**: when the request was received
- **Network context**: source/destination addresses (when implemented)
- **TLS information**: SNI, certificate details (when implemented)
- **Context extensions**: custom key-value pairs from your AgentGateway config

## Building the Service

### Prerequisites

- Go 1.21 or later
- `protoc` (Protocol Buffers compiler)
- Go protobuf plugins: `protoc-gen-go` and `protoc-gen-go-grpc`

### Installing Dependencies

1. **Install protoc** (Protocol Buffers compiler):
   ```bash
   # On macOS with Homebrew:
   brew install protobuf
   
   # On Ubuntu/Debian:
   sudo apt-get install protobuf-compiler
   
   # On Windows with Chocolatey:
   choco install protoc
   ```

2. **Install Go protobuf plugins**:
   ```bash
   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
   ```

### Building Steps

1. **Generate protobuf code**:
   ```bash
   cd policy_engine
   make generate
   ```

2. **Install Go dependencies**:
   ```bash
   go mod tidy
   ```

3. **Build the service**:
   ```bash
   make build
   ```

### Alternative: Manual protoc command

If you prefer to run protoc manually instead of using make:

```bash
mkdir -p gen
protoc --go_out=gen --go_opt=paths=source_relative \
       --go-grpc_out=gen --go-grpc_opt=paths=source_relative \
       proto/ext_authz.proto
```

## Running the Service

### Basic Usage

```bash
make run
```

This builds and starts the service on port 8080 (default).

### Custom Port

```bash
./policy-engine -port 9090
```

## Configuring AgentGateway

Add this to your AgentGateway configuration to use the policy engine:

```yaml
policies:
  extAuthz:
    target: "localhost:8080"  # or whatever port you're using
    context:  # Optional context extensions
      environment: "production"
      region: "us-west-1"
```

## Testing the Service

### Test Policies

1. **Admin access denied**:
   ```bash
   curl -X GET http://your-backend/admin/users
   # Should be denied with 403 Forbidden
   ```

2. **POST without auth denied**:
   ```bash
   curl -X POST http://your-backend/api/users
   # Should be denied - missing Authorization header
   ```

3. **Business hours restriction**:
   - Requests outside 9 AM - 5 PM will be denied
   - Check the logs for the specific time restriction message

4. **Production environment restriction**:
   - If `environment: "production"` is set in context extensions
   - Requires `x-production-access: true` header

5. **Bot user agents blocked**:
   ```bash
   curl -H "User-Agent: SomeBot/1.0" http://your-backend/api/health
   # Should be denied
   ```

### Successful Requests

Requests that meet all policy requirements will:
- Be allowed to proceed to the backend
- Have `x-authorized-by: policy-engine` header added
- Have `x-decision-time` header added with timestamp
- Have `authorization` header removed (for security)
- Include environment and region headers if specified in context extensions

## Logs

The service logs all authorization decisions and the full context received from AgentGateway. Watch the logs to see:

- What context AgentGateway sends
- Which policies are triggered
- Authorization decisions and reasons
- Header modifications

## Example Output

```
2024/01/15 10:30:00 Policy Engine starting on port 8080
2024/01/15 10:30:00 This service implements the Envoy ext_authz protocol
2024/01/15 10:30:00 Configure AgentGateway to use: ext_authz: { target: 'localhost:8080' }
2024/01/15 10:30:15 Received authorization request
2024/01/15 10:30:15 === REQUEST CONTEXT ===
2024/01/15 10:30:15 Method: GET
2024/01/15 10:30:15 Path: /api/users
2024/01/15 10:30:15 Host: example.com
2024/01/15 10:30:15 Scheme: https
2024/01/15 10:30:15 Protocol: HTTP/2
2024/01/15 10:30:15 Body size: 0
2024/01/15 10:30:15 Body: 
2024/01/15 10:30:15 Headers:
2024/01/15 10:30:15   user-agent: Mozilla/5.0...
2024/01/15 10:30:15   authorization: Bearer token123
2024/01/15 10:30:15 Context Extensions:
2024/01/15 10:30:15   environment: production
2024/01/15 10:30:15   region: us-west-1
2024/01/15 10:30:15 =======================
2024/01/15 10:30:15 Request ALLOWED: request meets all policy requirements
```

## Next Steps

This is a simple sample to test connectivity. For production use, consider:

1. **Replacing hardcoded policies** with a proper policy engine (OPA, Casbin, etc.)
2. **Adding authentication** to the gRPC service itself
3. **Implementing caching** for performance
4. **Adding metrics and monitoring**
5. **Supporting dynamic policy updates**

## Troubleshooting

### Common Issues

1. **Import errors**: Make sure you've run `make generate` first
2. **Port conflicts**: Use `-port` flag to specify a different port
3. **gRPC errors**: Ensure AgentGateway is configured with the correct target address
4. **protoc not found**: Install Protocol Buffers compiler (see Prerequisites section)
5. **Go protobuf plugins missing**: Run the go install commands for protoc-gen-go and protoc-gen-go-grpc

### Debug Mode

The service logs all context by default. If you need more verbose logging, you can modify the `logRequestContext` function in `main.go`.

### Checking Dependencies

```bash
# Check if protoc is installed
which protoc

# Check if Go protobuf plugins are installed
which protoc-gen-go
which protoc-gen-go-grpc
```
