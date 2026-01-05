package main

import (
	"encoding/json"
	"fmt"
	"log"

	pb "policy_engine/gen/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// extractPrincipal extracts the user principal from JWT claims in metadataContext
// Returns the principal in the format "user:{preferred_username}"
func extractPrincipal(attrs *pb.AttributeContext) (string, error) {
	metadataCtx := attrs.GetMetadataContext()
	if metadataCtx != nil {
		filterMetadata := metadataCtx.GetFilterMetadata()
		if filterMetadata != nil {
			log.Printf("DEBUG: Checking filter metadata, found %d entries", len(filterMetadata))
			for key := range filterMetadata {
				log.Printf("DEBUG: Filter metadata key: %s", key)
			}

			// Try Envoy JWT filter format: envoy.filters.http.jwt_authn with nested jwt_payload
			if jwtAuthnData, ok := filterMetadata["envoy.filters.http.jwt_authn"]; ok {
				log.Printf("DEBUG: Found envoy.filters.http.jwt_authn metadata")
				jwtAuthnMap := jwtAuthnData.AsMap()
				log.Printf("DEBUG: jwt_authn map keys: %v", getMapKeys(jwtAuthnMap))
				
				if jwtPayload, ok := jwtAuthnMap["jwt_payload"]; ok {
					log.Printf("DEBUG: Found jwt_payload, type: %T", jwtPayload)
					// jwt_payload can be a map[string]interface{} or a structpb.Struct
					var claimsMap map[string]interface{}
					if payloadMap, ok := jwtPayload.(map[string]interface{}); ok {
						claimsMap = payloadMap
					} else if payloadStruct, ok := jwtPayload.(*structpb.Struct); ok {
						claimsMap = payloadStruct.AsMap()
					} else {
						// Try to convert if it's wrapped in another way
						log.Printf("DEBUG: jwt_payload is unexpected type: %T", jwtPayload)
						claimsMap = make(map[string]interface{})
					}

					log.Printf("DEBUG: Claims map keys: %v", getMapKeys(claimsMap))

					// Try to get preferred_username
					if username, ok := claimsMap["preferred_username"]; ok {
						if usernameStr, ok := username.(string); ok && usernameStr != "" {
							log.Printf("DEBUG: Extracted preferred_username: %s", usernameStr)
							return fmt.Sprintf("user:%s", usernameStr), nil
						}
					}

					// Fallback to sub (subject)
					if sub, ok := claimsMap["sub"]; ok {
						if subStr, ok := sub.(string); ok && subStr != "" {
							log.Printf("DEBUG: Extracted sub: %s", subStr)
							return fmt.Sprintf("user:%s", subStr), nil
						}
					}
				} else {
					log.Printf("DEBUG: jwt_payload not found in jwt_authn map")
				}
			} else {
				log.Printf("DEBUG: envoy.filters.http.jwt_authn not found in filter metadata")
			}

			// Fallback: Try agentgateway format: agentgateway.jwt.claims (flat structure)
			if jwtClaims, ok := filterMetadata["agentgateway.jwt.claims"]; ok {
				log.Printf("DEBUG: Found agentgateway.jwt.claims metadata")
				claimsMap := jwtClaims.AsMap()
				log.Printf("DEBUG: Claims map keys: %v", getMapKeys(claimsMap))

				// Try to get preferred_username
				if username, ok := claimsMap["preferred_username"]; ok {
					if usernameStr, ok := username.(string); ok && usernameStr != "" {
						log.Printf("DEBUG: Extracted preferred_username: %s", usernameStr)
						return fmt.Sprintf("user:%s", usernameStr), nil
					}
				}

				// Fallback to sub (subject)
				if sub, ok := claimsMap["sub"]; ok {
					if subStr, ok := sub.(string); ok && subStr != "" {
						log.Printf("DEBUG: Extracted sub: %s", subStr)
						return fmt.Sprintf("user:%s", subStr), nil
					}
				}
			} else {
				log.Printf("DEBUG: agentgateway.jwt.claims not found in filter metadata")
			}
		} else {
			log.Printf("DEBUG: filterMetadata is nil")
		}
	} else {
		log.Printf("DEBUG: metadataCtx is nil")
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

// Helper function to get keys from a map for debugging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
