# Policy Engine - External Authorization Service with OpenFGA

This is an external authorization service that implements the Envoy ext_authz protocol integrated with OpenFGA for fine-grained authorization. It's designed to work with AgentGateway to provide relationship-based access control (ReBAC) for API requests.

## Features

The policy engine integrates with OpenFGA to provide:

1. **Fine-grained authorization**: Uses OpenFGA for relationship-based access control
2. **JWT-based principal extraction**: Extracts user identity from JWT claims in request metadata
3. **Resource-based authorization**: Extracts model names from OpenAI API request bodies
4. **Flexible relation checking**: Configurable relation to check (e.g., `can_access`, `viewer`, `editor`)
5. **Fail-closed security**: Denies all requests if OpenFGA is unavailable
6. **Header enrichment**: Adds authorization metadata to allowed requests

## How It Works

1. **AgentGateway** receives an HTTP request and calls this ext_authz service via gRPC
2. The service extracts the **principal** (user) from JWT claims in `metadataContext.filterMetadata["agentgateway.jwt.claims"].preferred_username`
3. The service extracts the **resource** (model) from the OpenAI API request body JSON field `model`
4. The service calls **OpenFGA Check API** with: `user:{username}`, `relation:{configured_relation}`, `object:model:{model_name}`
5. If OpenFGA returns `allowed: true`, the request is **allowed** and forwarded to the backend
6. If OpenFGA returns `allowed: false` or errors occur, the request is **denied** with 403 Forbidden

## What Context Does AgentGateway Send?

The service receives rich context from AgentGateway:

- **HTTP Request**: method, path, host, scheme, protocol, headers, body
- **JWT Claims**: In `metadataContext.filterMetadata["agentgateway.jwt.claims"]` including:
  - `preferred_username`: Username for principal extraction
  - `sub`: User ID (alternative principal identifier)
  - `realm_access.roles`: User roles from Keycloak
  - `email`, `name`, and other claims
- **Network context**: source/destination addresses
- **Context extensions**: custom key-value pairs from your AgentGateway config (environment, region, service)

## Prerequisites

1. **Go 1.21 or later**
2. **Protocol Buffers compiler** (`protoc`)
3. **Go protobuf plugins**: `protoc-gen-go` and `protoc-gen-go-grpc`
4. **OpenFGA server** running and accessible

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

3. **Install OpenFGA CLI** (optional, for testing):
   ```bash
   brew install openfga/tap/fga  # macOS
   # Or download from https://github.com/openfga/cli/releases
   ```

## Setting Up OpenFGA

### 1. Start OpenFGA Server

```bash
# Using Docker
docker run -p 8080:8080 openfga/openfga run

# Or using Docker Compose (recommended)
# Create docker-compose.yml:
cat > docker-compose.yml <<EOF
version: '3.8'
services:
  openfga:
    image: openfga/openfga:latest
    ports:
      - "8080:8080"
    command: run
EOF

docker-compose up -d
```

### 2. Create an OpenFGA Store

```bash
# Using the FGA CLI
fga store create --name "ai-gateway-authz"

# Or using curl
curl -X POST http://localhost:8080/stores \
  -H "Content-Type: application/json" \
  -d '{"name": "ai-gateway-authz"}'

# Save the store_id from the response
```

### 3. Define Authorization Model

Create a file `model.fga` with your authorization model:

```
model
  schema 1.1

type user

type model
  relations
    define owner: [user]
    define can_access: [user] or owner
```

Upload the model:

```bash
# Using FGA CLI
fga model write --store-id=<YOUR_STORE_ID> --file=model.fga

# Or using curl
curl -X POST "http://localhost:8080/stores/<YOUR_STORE_ID>/authorization-models" \
  -H "Content-Type: application/json" \
  -d '{
    "type_definitions": [
      {
        "type": "user"
      },
      {
        "type": "model",
        "relations": {
          "owner": {
            "this": {}
          },
          "can_access": {
            "union": {
              "child": [
                {"this": {}},
                {"computedUserset": {"relation": "owner"}}
              ]
            }
          }
        }
      }
    ]
  }'

# Save the authorization_model_id from the response
```

### 4. Add Relationship Tuples

Add tuples to grant users access to models:

```bash
# Using FGA CLI
fga tuple write --store-id=<YOUR_STORE_ID> \
  user:mcp-user can_access model:gpt-4

# Or using curl
curl -X POST "http://localhost:8080/stores/<YOUR_STORE_ID>/write" \
  -H "Content-Type: application/json" \
  -d '{
    "writes": {
      "tuple_keys": [
        {
          "user": "user:mcp-user",
          "relation": "can_access",
          "object": "model:gpt-4"
        },
        {
          "user": "user:mcp-user",
          "relation": "can_access",
          "object": "model:gpt-3.5-turbo"
        }
      ]
    }
  }'
```

## Building the Service

### 1. Generate protobuf code

```bash
make generate
```

### 2. Install Go dependencies

```bash
go mod tidy
```

### 3. Build the service

```bash
make build
```

### Alternative: Manual protoc command

If you prefer to run protoc manually:

```bash
mkdir -p gen/proto
protoc --go_out=gen --go_opt=paths=source_relative \
       --go-grpc_out=gen --go-grpc_opt=paths=source_relative \
       proto/ext_authz.proto
```

## Configuration

Create a `.env` file from the example:

```bash
cp .env.example .env
```

Edit `.env` and set your OpenFGA connection details:

```bash
OPENFGA_API_URL=http://localhost:8080
OPENFGA_STORE_ID=<your-store-id>
OPENFGA_MODEL_ID=<your-model-id>  # Optional: uses latest if omitted
OPENFGA_RELATION=can_access
```

Or set environment variables directly:

```bash
export OPENFGA_API_URL=http://localhost:8080
export OPENFGA_STORE_ID=01HQXYZ123456789ABCDEF
export OPENFGA_RELATION=can_access
```

## Running the Service

### Basic Usage

```bash
# Load environment variables and run
source .env  # or export variables manually
./policy-engine

# Or using make
make run
```

The service starts on port 7070 by default.

### Custom Port

```bash
./policy-engine -port 9090
```

### Example Startup Logs

```
2024/01/15 10:30:00 Configuration loaded:
2024/01/15 10:30:00   OpenFGA API URL: http://localhost:8080
2024/01/15 10:30:00   OpenFGA Store ID: 01HQXYZ123456789ABCDEF
2024/01/15 10:30:00   OpenFGA Model ID:
2024/01/15 10:30:00   OpenFGA Relation: can_access
2024/01/15 10:30:00 OpenFGA client initialized: URL=http://localhost:8080, StoreID=01HQXYZ123456789ABCDEF, ModelID=
2024/01/15 10:30:00 Policy Engine starting on port 7070
2024/01/15 10:30:00 This service implements the Envoy ext_authz protocol with OpenFGA authorization
2024/01/15 10:30:00 Configure AgentGateway to use: ext_authz: { target: 'localhost:7070' }
```

## Configuring AgentGateway

Add this to your AgentGateway configuration to use the policy engine:

```yaml
binds:
  - port: 3001
    listeners:
      - routes:
          - policies:
              extAuthz:
                host: "localhost:7070"  # Match the port from policy-engine
                context:  # Optional context extensions
                  environment: "development"
                  region: "us-west-1"
                  service: "agentgateway"
```

The AgentGateway should be configured to:
1. **Extract JWT claims** and pass them in `metadataContext.filterMetadata["agentgateway.jwt.claims"]`
2. **Include request body** in the ext_authz call (needed for model extraction)

## Testing the Service

### Test Authorization with OpenFGA

**Scenario 1: User has access to model**

```bash
# First, add the tuple in OpenFGA
fga tuple write --store-id=<YOUR_STORE_ID> \
  user:mcp-user can_access model:gpt-4

# Then make a request through AgentGateway
curl -X POST http://localhost:3001/opa/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <jwt-with-preferred-username-mcp-user>" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello"}]
  }'

# Expected: Request is ALLOWED
# Logs will show:
# - Extracted principal: user:mcp-user
# - Extracted resource: model:gpt-4
# - OpenFGA Check result: allowed=true
```

**Scenario 2: User does NOT have access to model**

```bash
# Make a request for a model the user doesn't have access to
curl -X POST http://localhost:3001/opa/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <jwt-with-preferred-username-mcp-user>" \
  -d '{
    "model": "gpt-4-turbo",
    "messages": [{"role": "user", "content": "Hello"}]
  }'

# Expected: Request is DENIED with 403 Forbidden
# Response body: "Access Denied: User user:mcp-user does not have can_access permission for model:gpt-4-turbo"
```

**Scenario 3: Testing OpenFGA check directly**

You can test OpenFGA checks directly using the FGA CLI:

```bash
# Check if user:mcp-user has can_access on model:gpt-4
fga query check --store-id=<YOUR_STORE_ID> \
  user:mcp-user can_access model:gpt-4

# Expected output: {"allowed":true}
```

### Successful Requests

Requests that are allowed will:
- Be forwarded to the backend
- Have `x-authorized-by: openfga-policy-engine` header added
- Have `x-decision-time` header added with timestamp
- Have `x-authorized-user` header added with the principal
- Include environment and region headers if specified in context extensions

### Denied Requests

Requests that are denied will:
- Return HTTP 403 Forbidden
- Include `x-auth-denied: true` header
- Include `x-auth-reason` header with the denial reason
- Have a response body explaining the denial

## Logs

The service logs all authorization decisions and the full context received from AgentGateway:

### Example Authorization Flow (Allowed)

```
2024/01/15 10:30:15 Received authorization request
2024/01/15 10:30:15 === REQUEST CONTEXT ===
2024/01/15 10:30:15 Method: POST
2024/01/15 10:30:15 Path: /opa/openai/v1/chat/completions
2024/01/15 10:30:15 Host: localhost
2024/01/15 10:30:15 Scheme: http
2024/01/15 10:30:15 Body size: 116
2024/01/15 10:30:15 Body: {"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}
2024/01/15 10:30:15 Headers:
2024/01/15 10:30:15   content-type: application/json
2024/01/15 10:30:15   user-agent: curl/8.7.1
2024/01/15 10:30:15 Context Extensions:
2024/01/15 10:30:15   environment: development
2024/01/15 10:30:15   region: us-west-1
2024/01/15 10:30:15   service: agentgateway
2024/01/15 10:30:15 =======================
2024/01/15 10:30:15 Extracted principal: user:mcp-user
2024/01/15 10:30:15 Extracted resource: model:gpt-4
2024/01/15 10:30:15 OpenFGA Check: user=user:mcp-user, relation=can_access, object=model:gpt-4
2024/01/15 10:30:15 OpenFGA Check result: allowed=true
2024/01/15 10:30:15 Request ALLOWED: OpenFGA authorization granted: user:mcp-user has can_access on model:gpt-4
```

### Example Authorization Flow (Denied)

```
2024/01/15 10:31:00 Received authorization request
2024/01/15 10:31:00 === REQUEST CONTEXT ===
2024/01/15 10:31:00 Method: POST
2024/01/15 10:31:00 Path: /opa/openai/v1/chat/completions
2024/01/15 10:31:00 Body: {"model":"gpt-4-turbo","messages":[...]}
2024/01/15 10:31:00 =======================
2024/01/15 10:31:00 Extracted principal: user:mcp-user
2024/01/15 10:31:00 Extracted resource: model:gpt-4-turbo
2024/01/15 10:31:00 OpenFGA Check: user=user:mcp-user, relation=can_access, object=model:gpt-4-turbo
2024/01/15 10:31:00 OpenFGA Check result: allowed=false
2024/01/15 10:31:00 Request DENIED: User user:mcp-user does not have can_access permission for model:gpt-4-turbo
```

## Example OpenFGA Authorization Model

Here's a more complete authorization model for AI model access control:

```
model
  schema 1.1

type user

type group
  relations
    define member: [user]

type model
  relations
    define owner: [user, group#member]
    define editor: [user, group#member]
    define viewer: [user, group#member]
    define can_access: viewer or editor or owner
```

Example tuples:

```bash
# Grant direct user access
fga tuple write user:alice can_access model:gpt-4

# Grant group-based access
fga tuple write user:bob member group:ai-team
fga tuple write group:ai-team#member viewer model:gpt-4

# Grant ownership
fga tuple write user:admin owner model:gpt-4-turbo
```

## Production Considerations

For production deployments, consider:

1. **Caching**: Add response caching (5-60s TTL) to reduce OpenFGA API calls
2. **Circuit breaker**: Implement circuit breaker pattern for OpenFGA calls
3. **Metrics**: Add Prometheus metrics for authorization decisions, latency, errors
4. **Observability**: Integrate with distributed tracing (OpenTelemetry)
5. **Resource extraction**: Extend to support more resource types beyond models
6. **Multi-tenancy**: Add organization/tenant scoping to authorization checks
7. **Audit logging**: Send all authorization decisions to audit log system
8. **Testing**: Add unit and integration tests with mock OpenFGA server

## Troubleshooting

### Common Issues

1. **"OPENFGA_STORE_ID environment variable is required"**
   - Make sure you've set the `OPENFGA_STORE_ID` environment variable
   - Run `source .env` if using .env file
   - Or export manually: `export OPENFGA_STORE_ID=<your-store-id>`

2. **"OpenFGA check failed: connection refused"**
   - Ensure OpenFGA server is running: `docker ps | grep openfga`
   - Check the OpenFGA API URL is correct: `OPENFGA_API_URL=http://localhost:8080`
   - Verify connectivity: `curl http://localhost:8080/healthz`

3. **"Failed to extract principal: no JWT claims found"**
   - AgentGateway must extract JWT and pass claims in metadata
   - Check AgentGateway configuration for JWT extraction
   - Ensure requests include valid JWT tokens

4. **"Failed to extract resource: request body is empty"**
   - AgentGateway must include request body in ext_authz calls
   - Check AgentGateway configuration to enable body passing
   - Ensure requests have JSON body with `model` field

5. **"User does not have permission" (but you added the tuple)**
   - Verify tuple was added: `fga tuple read --store-id=<STORE_ID>`
   - Check exact formatting: `user:mcp-user` (not `mcp-user`)
   - Check model name matches: `model:gpt-4` (case-sensitive)
   - Verify using correct store ID and model ID

6. **Import errors**
   - Run `make generate` to generate protobuf code
   - Run `go mod tidy` to install dependencies

7. **Port conflicts**
   - Use `-port` flag: `./policy-engine -port 9090`
   - Update AgentGateway config to match the new port

### Debug Mode

The service logs all context by default. To see more details:

```bash
# Watch logs in real-time
./policy-engine 2>&1 | tee policy-engine.log

# Check what principal/resource are extracted
grep "Extracted" policy-engine.log

# Check OpenFGA check results
grep "OpenFGA Check" policy-engine.log
```

### Testing OpenFGA Directly

Bypass the ext_authz service to test OpenFGA:

```bash
# Check authorization directly
fga query check --store-id=<YOUR_STORE_ID> \
  user:mcp-user can_access model:gpt-4

# List all tuples
fga tuple read --store-id=<YOUR_STORE_ID>

# List models
fga model list --store-id=<YOUR_STORE_ID>
```

### Checking Dependencies

```bash
# Check if protoc is installed
which protoc

# Check if Go protobuf plugins are installed
which protoc-gen-go
which protoc-gen-go-grpc

# Check OpenFGA server
curl http://localhost:8080/healthz

# Check FGA CLI
fga version
```

## Architecture Diagram

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │ HTTP Request (with JWT)
       │ POST /opa/openai/v1/chat/completions
       │ Body: {"model": "gpt-4", ...}
       ↓
┌──────────────────┐
│  AgentGateway    │
│                  │
│ 1. Extract JWT   │
│ 2. Parse claims  │
└──────┬───────────┘
       │ gRPC ext_authz.Check()
       │ - metadataContext.filterMetadata["agentgateway.jwt.claims"]
       │ - request.http.body
       ↓
┌──────────────────────────┐
│  Policy Engine (This)    │
│                          │
│ 1. Extract principal     │ ← user:mcp-user
│ 2. Extract resource      │ ← model:gpt-4
│ 3. Call OpenFGA Check    │
└──────┬───────────────────┘
       │ HTTP POST /stores/{store_id}/check
       │ Body: {user, relation, object}
       ↓
┌──────────────────┐
│    OpenFGA       │
│                  │
│ Evaluate tuples  │
│ and model        │
└──────┬───────────┘
       │ Response: {"allowed": true/false}
       ↓
┌──────────────────────────┐
│  Policy Engine           │
│  Decision: ALLOW/DENY    │
└──────┬───────────────────┘
       │ gRPC CheckResponse
       ↓
┌──────────────────┐
│  AgentGateway    │
│  Forward/Reject  │
└──────┬───────────┘
       │ HTTP Response
       ↓
┌─────────────┐
│   Client    │
└─────────────┘
```
