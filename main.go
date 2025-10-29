package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "policy_engine/gen/proto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

var (
	port = flag.Int("port", 7070, "The server port")
)

type authorizationServer struct {
	pb.UnimplementedAuthorizationServer
	openfgaClient *OpenFGAClient
	config        *Config
}

// Check implements the Authorization service Check method
func (s *authorizationServer) Check(ctx context.Context, req *pb.CheckRequest) (*pb.CheckResponse, error) {
	log.Printf("Received authorization request")

	// Extract the request context
	attrs := req.GetAttributes()
	if attrs == nil {
		return nil, status.Error(codes.InvalidArgument, "missing attributes")
	}

	// Log all the context we receive from AgentGateway
	logRequestContext(attrs)

	// Make authorization decision based on OpenFGA
	decision := s.evaluatePolicy(ctx, attrs)

	if decision.allowed {
		log.Printf("Request ALLOWED: %s", decision.reason)
		return buildAllowResponse(decision), nil
	} else {
		log.Printf("Request DENIED: %s", decision.reason)
		return buildDenyResponse(decision), nil
	}
}

type policyDecision struct {
	allowed         bool
	reason          string
	headers         map[string]string
	headersToRemove []string
}

func logRequestContext(attrs *pb.AttributeContext) {
	log.Printf("=== REQUEST CONTEXT ===")

	// HTTP Request details
	if req := attrs.GetRequest(); req != nil {
		if httpReq := req.GetHttp(); httpReq != nil {
			log.Printf("Method: %s", httpReq.GetMethod())
			log.Printf("Path: %s", httpReq.GetPath())
			log.Printf("Host: %s", httpReq.GetHost())
			log.Printf("Scheme: %s", httpReq.GetScheme())
			log.Printf("Body size: %d", httpReq.GetSize())
			log.Printf("Body: %s", httpReq.GetBody())

			// Log all headers
			log.Printf("Headers:")
			for key, value := range httpReq.GetHeaders() {
				log.Printf("  %s: %s", key, value)
			}
		}

		// Log timing
		if req.GetTime() != nil {
			log.Printf("Request time: %v", req.GetTime().AsTime())
		}
	}

	// Source and destination info
	if attrs.GetSource() != nil {
		log.Printf("Source: %v", attrs.GetSource())
	}
	if attrs.GetDestination() != nil {
		log.Printf("Destination: %v", attrs.GetDestination())
	}

	// Context extensions (custom metadata from AgentGateway config)
	if len(attrs.GetContextExtensions()) > 0 {
		log.Printf("Context Extensions:")
		for key, value := range attrs.GetContextExtensions() {
			log.Printf("  %s: %s", key, value)
		}
	}

	// TLS session info
	if attrs.GetTlsSession() != nil {
		log.Printf("TLS SNI: %s", attrs.GetTlsSession().GetSni())
	}

	log.Printf("=======================")
}

func (s *authorizationServer) evaluatePolicy(ctx context.Context, attrs *pb.AttributeContext) policyDecision {
	// Extract principal (user) from JWT claims
	principal, err := extractPrincipal(attrs)
	if err != nil {
		log.Printf("Failed to extract principal: %v", err)
		return policyDecision{
			allowed: false,
			reason:  fmt.Sprintf("Failed to extract principal: %v", err),
		}
	}

	log.Printf("Extracted principal: %s", principal)

	// Extract resource (model) from request body
	resource, err := extractResource(attrs)
	if err != nil {
		log.Printf("Failed to extract resource: %v", err)
		return policyDecision{
			allowed: false,
			reason:  fmt.Sprintf("Failed to extract resource: %v", err),
		}
	}

	log.Printf("Extracted resource: %s", resource)

	// Perform OpenFGA authorization check
	checkParams := CheckParams{
		User:     principal,
		Relation: s.config.OpenFGA.Relation,
		Object:   resource,
	}

	allowed, err := s.openfgaClient.Check(ctx, checkParams)
	if err != nil {
		// Fail closed: deny access if OpenFGA check fails
		log.Printf("OpenFGA check error: %v", err)
		return policyDecision{
			allowed: false,
			reason:  fmt.Sprintf("Authorization check failed: %v", err),
		}
	}

	if !allowed {
		return policyDecision{
			allowed: false,
			reason: fmt.Sprintf("User %s does not have %s permission for %s",
				principal, s.config.OpenFGA.Relation, resource),
		}
	}

	// Request is allowed
	contextExts := attrs.GetContextExtensions()
	decision := policyDecision{
		allowed: true,
		reason:  fmt.Sprintf("OpenFGA authorization granted: %s has %s on %s", principal, s.config.OpenFGA.Relation, resource),
		headers: map[string]string{
			"x-authorized-by":   "openfga-policy-engine",
			"x-decision-time":   time.Now().Format(time.RFC3339),
			"x-authorized-user": principal,
		},
		headersToRemove: []string{}, // Keep headers by default
	}

	// Add environment-specific headers from context extensions
	if env, exists := contextExts["environment"]; exists {
		decision.headers["x-environment"] = env
	}

	// Add region info if available
	if region, exists := contextExts["region"]; exists {
		decision.headers["x-region"] = region
	}

	return decision
}

func buildAllowResponse(decision policyDecision) *pb.CheckResponse {
	// Build headers to add/modify
	var headers []*pb.HeaderValueOption
	for key, value := range decision.headers {
		headers = append(headers, &pb.HeaderValueOption{
			Header: &pb.HeaderValue{
				Key:   key,
				Value: value,
			},
			Append: &wrapperspb.BoolValue{Value: false}, // Replace existing headers
		})
	}

	// Build headers to remove
	var headersToRemove []string
	for _, header := range decision.headersToRemove {
		headersToRemove = append(headersToRemove, header)
	}

	return &pb.CheckResponse{
		Status: &pb.Status{
			Code:    0, // 0 = OK (allow)
			Message: decision.reason,
		},
		HttpResponse: &pb.CheckResponse_OkResponse{
			OkResponse: &pb.OkHttpResponse{
				Headers:              headers,
				HeadersToRemove:      headersToRemove,
				ResponseHeadersToAdd: []*pb.HeaderValueOption{}, // No response headers to add
			},
		},
	}
}

func buildDenyResponse(decision policyDecision) *pb.CheckResponse {
	return &pb.CheckResponse{
		Status: &pb.Status{
			Code:    7, // 7 = PERMISSION_DENIED
			Message: decision.reason,
		},
		HttpResponse: &pb.CheckResponse_DeniedResponse{
			DeniedResponse: &pb.DeniedHttpResponse{
				Status: &pb.HttpStatus{
					Code: pb.StatusCode_Forbidden, // 403 Forbidden
				},
				Headers: []*pb.HeaderValueOption{
					{
						Header: &pb.HeaderValue{
							Key:   "x-auth-denied",
							Value: "true",
						},
					},
					{
						Header: &pb.HeaderValue{
							Key:   "x-auth-reason",
							Value: decision.reason,
						},
					},
				},
				Body: fmt.Sprintf("Access Denied: %s", decision.reason),
			},
		},
	}
}

func main() {
	flag.Parse()

	// Load configuration
	config, err := LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded:")
	log.Printf("  OpenFGA API URL: %s", config.OpenFGA.APIURL)
	log.Printf("  OpenFGA Store ID: %s", config.OpenFGA.StoreID)
	log.Printf("  OpenFGA Model ID: %s", config.OpenFGA.AuthorizationModelID)
	log.Printf("  OpenFGA Relation: %s", config.OpenFGA.Relation)

	// Initialize OpenFGA client
	openfgaClient, err := NewOpenFGAClient(config.OpenFGA)
	if err != nil {
		log.Fatalf("Failed to create OpenFGA client: %v", err)
	}

	// Use the port from flag if provided, otherwise use config default
	if *port != 7070 {
		config.Port = *port
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterAuthorizationServer(s, &authorizationServer{
		openfgaClient: openfgaClient,
		config:        config,
	})

	log.Printf("Policy Engine starting on port %d", config.Port)
	log.Printf("This service implements the Envoy ext_authz protocol with OpenFGA authorization")
	log.Printf("Configure AgentGateway to use: ext_authz: { target: 'localhost:%d' }", config.Port)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
