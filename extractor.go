package main

import (
	"encoding/json"
	"fmt"

	pb "policy_engine/gen/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// extractPrincipal extracts the user principal from JWT claims in metadataContext
// Returns the principal in the format "user:{preferred_username}"
func extractPrincipal(attrs *pb.AttributeContext) (string, error) {
	// Try to extract from metadataContext.filterMetadata["agentgateway.jwt.claims"]
	metadataCtx := attrs.GetMetadataContext()
	if metadataCtx != nil {
		filterMetadata := metadataCtx.GetFilterMetadata()
		if filterMetadata != nil {
			// Get JWT claims from agentgateway
			if jwtClaims, ok := filterMetadata["agentgateway.jwt.claims"]; ok {
				claimsMap := jwtClaims.AsMap()

				// Try to get preferred_username
				if username, ok := claimsMap["preferred_username"]; ok {
					if usernameStr, ok := username.(string); ok && usernameStr != "" {
						return fmt.Sprintf("user:%s", usernameStr), nil
					}
				}

				// Fallback to sub (subject)
				if sub, ok := claimsMap["sub"]; ok {
					if subStr, ok := sub.(string); ok && subStr != "" {
						return fmt.Sprintf("user:%s", subStr), nil
					}
				}
			}
		}
	}

	// Fallback 1: Try to get user from context extensions
	contextExts := attrs.GetContextExtensions()
	if contextExts != nil {
		if username, ok := contextExts["user"]; ok && username != "" {
			return fmt.Sprintf("user:%s", username), nil
		}
	}

	// Fallback 2: Try to extract from x-user header
	req := attrs.GetRequest()
	if req != nil {
		if httpReq := req.GetHttp(); httpReq != nil {
			headers := httpReq.GetHeaders()
			if username, ok := headers["x-user"]; ok && username != "" {
				return fmt.Sprintf("user:%s", username), nil
			}
		}
	}

	return "", fmt.Errorf("no user found in JWT claims, context_extensions, or x-user header")
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
