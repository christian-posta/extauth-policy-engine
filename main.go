package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
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

	// Make authorization decision based on the context
	decision := evaluatePolicy(attrs)

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

func evaluatePolicy(attrs *pb.AttributeContext) policyDecision {
	// Extract HTTP request details
	req := attrs.GetRequest()
	if req == nil {
		return policyDecision{allowed: false, reason: "missing request"}
	}

	httpReq := req.GetHttp()
	if httpReq == nil {
		return policyDecision{allowed: false, reason: "missing HTTP request"}
	}

	method := httpReq.GetMethod()
	path := httpReq.GetPath()
	headers := httpReq.GetHeaders()
	contextExts := attrs.GetContextExtensions()

	// Policy 1: Deny all requests to /admin/* paths
	if strings.HasPrefix(path, "/admin") {
		return policyDecision{
			allowed: false,
			reason:  fmt.Sprintf("access denied to admin path: %s", path),
		}
	}

	// Policy 2: Require Authorization header for POST requests
	if method == "POST" {
		if authHeader, exists := headers["authorization"]; !exists || authHeader == "" {
			return policyDecision{
				allowed: false,
				reason:  "POST requests require Authorization header",
			}
		}
	}

	// Policy 3: Business hours restriction (9 AM - 5 PM)
	now := time.Now()
	if now.Hour() < 9 || now.Hour() >= 17 {
		return policyDecision{
			allowed: false,
			reason:  fmt.Sprintf("access restricted to business hours (9 AM - 5 PM), current time: %s", now.Format("3:04 PM")),
		}
	}

	// Policy 4: Environment-based restrictions using context extensions
	if env, exists := contextExts["environment"]; exists {
		if env == "production" {
			// In production, require special header
			if specialHeader, exists := headers["x-production-access"]; !exists || specialHeader != "true" {
				return policyDecision{
					allowed: false,
					reason:  "production environment requires x-production-access header",
				}
			}
		}
	}

	// Policy 5: Rate limiting based on user agent (simple example)
	if userAgent, exists := headers["user-agent"]; exists {
		if strings.Contains(strings.ToLower(userAgent), "bot") {
			return policyDecision{
				allowed: false,
				reason:  "bot user agents are not allowed",
			}
		}
	}

	// If we get here, the request is allowed
	decision := policyDecision{
		allowed: true,
		reason:  "request meets all policy requirements",
		headers: map[string]string{
			"x-authorized-by": "policy-engine",
			"x-decision-time": time.Now().Format(time.RFC3339),
		},
		headersToRemove: []string{"authorization"}, // Remove auth header before sending to backend
	}

	// Add environment-specific headers
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

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterAuthorizationServer(s, &authorizationServer{})

	log.Printf("Policy Engine starting on port %d", *port)
	log.Printf("This service implements the Envoy ext_authz protocol")
	log.Printf("Configure AgentGateway to use: ext_authz: { target: 'localhost:%d' }", *port)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
