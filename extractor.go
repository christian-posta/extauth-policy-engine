package main

import (
	"encoding/json"
	"fmt"

	pb "policy_engine/gen/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// extractPrincipal extracts the user principal from context extensions or headers
// Returns the principal in the format "user:{preferred_username}"
//
// NOTE: This is a simplified implementation. In production, you would extract
// the JWT claims from metadataContext.filterMetadata["agentgateway.jwt.claims"].preferred_username
// For now, we use context_extensions as a workaround. You can set this in AgentGateway config:
//   context:
//     user: "mcp-user"  # or extract from JWT
func extractPrincipal(attrs *pb.AttributeContext) (string, error) {
	// Try to get user from context extensions
	contextExts := attrs.GetContextExtensions()
	if contextExts != nil {
		if username, ok := contextExts["user"]; ok && username != "" {
			return fmt.Sprintf("user:%s", username), nil
		}
	}

	// Fallback: try to extract from x-user header
	req := attrs.GetRequest()
	if req != nil {
		if httpReq := req.GetHttp(); httpReq != nil {
			headers := httpReq.GetHeaders()
			if username, ok := headers["x-user"]; ok && username != "" {
				return fmt.Sprintf("user:%s", username), nil
			}
		}
	}

	return "", fmt.Errorf("no user found in context_extensions['user'] or x-user header")
}

// OpenAIRequest represents the structure of an OpenAI API request
type OpenAIRequest struct {
	Model string `json:"model"`
	// Other fields omitted for now, can be added as needed
}

// extractResource extracts the model name from the OpenAI API request body
// Returns the resource in the format "model:{model_name}"
func extractResource(attrs *pb.AttributeContext) (string, error) {
	req := attrs.GetRequest()
	if req == nil {
		return "", fmt.Errorf("no request found in context")
	}

	httpReq := req.GetHttp()
	if httpReq == nil {
		return "", fmt.Errorf("no HTTP request found")
	}

	// Get the request body - prefer raw_body (bytes) over body (string) to avoid UTF-8 issues
	var bodyBytes []byte
	rawBody := httpReq.GetRawBody()
	if len(rawBody) > 0 {
		bodyBytes = rawBody
	} else {
		// Fallback to string body if raw body is empty
		body := httpReq.GetBody()
		if body == "" {
			return "", fmt.Errorf("request body is empty")
		}
		bodyBytes = []byte(body)
	}

	// Parse the JSON body to extract the model field
	var openAIReq OpenAIRequest
	if err := json.Unmarshal(bodyBytes, &openAIReq); err != nil {
		return "", fmt.Errorf("failed to parse request body as JSON: %w", err)
	}

	if openAIReq.Model == "" {
		return "", fmt.Errorf("model field is missing or empty in request body")
	}

	// Return in OpenFGA format: "model:{model_name}"
	return fmt.Sprintf("model:%s", openAIReq.Model), nil
}

// Helper function to pretty print structpb.Struct for debugging
func structToMap(s *structpb.Struct) map[string]interface{} {
	if s == nil {
		return nil
	}
	return s.AsMap()
}
