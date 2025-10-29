# OpenFGA Integration Implementation Summary

## What Was Implemented

This document summarizes the OpenFGA integration that was added to the ext_authz policy engine.

### Files Created

1. **`config.go`** - Configuration management
   - Loads OpenFGA settings from environment variables
   - Required: `OPENFGA_STORE_ID`
   - Optional: `OPENFGA_API_URL`, `OPENFGA_MODEL_ID`, `OPENFGA_RELATION`

2. **`extractor.go`** - Principal and resource extraction
   - `extractPrincipal()` - Extracts user from context_extensions or x-user header
   - `extractResource()` - Extracts model name from OpenAI API request body JSON

3. **`openfga_client.go`** - OpenFGA SDK wrapper
   - `NewOpenFGAClient()` - Initializes OpenFGA client
   - `Check()` - Performs authorization checks
   - `BatchCheck()` - For future multi-resource checks
   - `ListObjects()` - List accessible objects
   - `ReadAuthorizationModel()` - For debugging

4. **`.env.example`** - Environment variable template
5. **`model.fga`** - Example OpenFGA authorization model

### Files Modified

1. **`main.go`**
   - Updated `authorizationServer` to include OpenFGA client and config
   - Changed `evaluatePolicy()` to use OpenFGA instead of hardcoded rules
   - Updated `main()` to initialize OpenFGA client from config

2. **`go.mod`**
   - Added `github.com/openfga/go-sdk v0.7.3` dependency

3. **`README.md`**
   - Completely rewritten with OpenFGA setup instructions
   - Added architecture diagram
   - Added testing scenarios
   - Added troubleshooting guide

## How It Works

### Request Flow

```
1. AgentGateway receives HTTP request with JWT
2. AgentGateway calls ext_authz Check() via gRPC
3. Policy engine extracts:
   - Principal: user from context_extensions["user"] or x-user header
   - Resource: model from request body JSON {"model": "..."}
4. Policy engine calls OpenFGA Check API:
   - User: user:{username}
   - Relation: can_access (configurable)
   - Object: model:{model_name}
5. OpenFGA evaluates tuples and returns allowed: true/false
6. Policy engine returns ALLOW (200) or DENY (403) to AgentGateway
7. AgentGateway forwards or rejects the request
```

### Principal Extraction (Current Implementation)

**Option 1: Context Extensions** (Recommended for testing)
```yaml
# In AgentGateway config:
context:
  user: "mcp-user"
```

**Option 2: HTTP Header**
```bash
curl -H "x-user: mcp-user" ...
```

**Future Enhancement:** Extract from JWT claims in `metadataContext.filterMetadata["agentgateway.jwt.claims"].preferred_username`
- Requires extended protobuf definition or custom metadata handling
- Current proto doesn't include metadataContext field

### Resource Extraction

Parses OpenAI API request body:
```json
{
  "model": "gpt-4",
  "messages": [...]
}
```

Extracts `"model"` field → becomes `"model:gpt-4"` in OpenFGA

## Configuration

### Environment Variables

```bash
OPENFGA_API_URL=http://localhost:8080        # OpenFGA server URL
OPENFGA_STORE_ID=01HQXYZ...                  # Required
OPENFGA_MODEL_ID=01HQABC...                  # Optional (uses latest)
OPENFGA_RELATION=can_access                  # Default relation to check
```

### AgentGateway Configuration

```yaml
binds:
  - port: 3001
    listeners:
      - routes:
          - policies:
              extAuthz:
                host: "localhost:7070"
                context:
                  user: "mcp-user"           # User for authorization
                  environment: "development"
                  region: "us-west-1"
```

## Example OpenFGA Setup

### 1. Authorization Model

```
model
  schema 1.1

type user

type model
  relations
    define owner: [user]
    define can_access: [user] or owner
```

### 2. Sample Tuples

```bash
# Grant user access to gpt-4
fga tuple write user:mcp-user can_access model:gpt-4

# Grant user access to gpt-3.5-turbo
fga tuple write user:mcp-user can_access model:gpt-3.5-turbo

# Grant ownership
fga tuple write user:admin owner model:gpt-4-turbo
```

## Testing

### Successful Authorization

```bash
# 1. Add tuple
fga tuple write --store-id=<STORE_ID> \
  user:mcp-user can_access model:gpt-4

# 2. Make request
curl -X POST http://localhost:3001/opa/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "x-user: mcp-user" \
  -d '{"model": "gpt-4", "messages": [...]}'

# Expected: 200 OK, request forwarded
```

### Failed Authorization

```bash
# Make request for model without access
curl -X POST http://localhost:3001/opa/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "x-user: mcp-user" \
  -d '{"model": "gpt-4-turbo", "messages": [...]}'

# Expected: 403 Forbidden
# Body: "Access Denied: User user:mcp-user does not have can_access permission for model:gpt-4-turbo"
```

## Known Limitations

1. **JWT Claims Extraction**: Currently simplified to use context_extensions or headers
   - Original plan was to extract from `metadataContext.filterMetadata`
   - Current protobuf definition doesn't include this field
   - Workaround: Pass user via context_extensions or x-user header
   - Future: Need extended protobuf or custom metadata handling

2. **Resource Extraction**: Only supports OpenAI API format
   - Hardcoded to extract `model` field from JSON body
   - Future: Support multiple API formats and resource types

3. **No Caching**: Every request calls OpenFGA API
   - Future: Add response caching with TTL

4. **No Circuit Breaker**: If OpenFGA is down, all requests fail
   - Current: Fail closed (deny all)
   - Future: Add circuit breaker pattern

## Future Enhancements

1. **JWT Integration**: Full JWT claims extraction from metadataContext
2. **Multiple Resource Types**: Support beyond just models (datasets, tools, etc.)
3. **Caching**: Add TTL-based cache for OpenFGA responses
4. **Circuit Breaker**: Graceful degradation when OpenFGA is unavailable
5. **Metrics**: Prometheus metrics for latency, error rates, cache hits
6. **Batch Operations**: Optimize multi-resource authorization checks
7. **Contextual Tuples**: Support runtime tuples from request context
8. **Audit Logging**: Centralized audit trail of all authorization decisions

## Build and Run

```bash
# Build
go build -o policy-engine main.go config.go extractor.go openfga_client.go

# Run
export OPENFGA_STORE_ID=<your-store-id>
export OPENFGA_API_URL=http://localhost:8080
./policy-engine

# Or with custom port
./policy-engine -port 9090
```

## Dependencies

- `github.com/openfga/go-sdk v0.7.3` - OpenFGA Go SDK
- `google.golang.org/grpc v1.64.0` - gRPC framework
- `google.golang.org/protobuf v1.33.0` - Protocol Buffers

## Architecture Decision Records

### Why Option 1 (Full Replacement)?

We chose to completely replace the hardcoded policy rules with OpenFGA for:
- **Single Source of Truth**: All authorization logic in OpenFGA
- **Flexibility**: Easy to update policies without code changes
- **Auditability**: OpenFGA provides audit trail of authorization decisions
- **Scalability**: Centralized authorization that can scale independently

### Why Context Extensions for User?

Initially planned to use JWT claims from metadataContext, but:
- Current protobuf definition doesn't include metadataContext field
- Would require updating proto file or custom handling
- Context extensions provide a working solution for MVP
- Can be enhanced later when proto is extended

### Fail Closed vs Fail Open

Chose fail closed (deny on error) because:
- Security-first approach
- Better to deny legitimate requests than allow unauthorized ones
- Can add circuit breaker later for graceful degradation
- Aligns with zero-trust security model
