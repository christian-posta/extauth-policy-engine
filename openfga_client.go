package main

import (
	"context"
	"fmt"
	"log"

	openfga "github.com/openfga/go-sdk"
	openfgaclient "github.com/openfga/go-sdk/client"
)

// OpenFGAClient wraps the OpenFGA SDK client
type OpenFGAClient struct {
	client *openfgaclient.OpenFgaClient
	config OpenFGAConfig
}

// NewOpenFGAClient creates a new OpenFGA client
func NewOpenFGAClient(config OpenFGAConfig) (*OpenFGAClient, error) {
	// Configure the OpenFGA client
	configuration := &openfgaclient.ClientConfiguration{
		ApiUrl:               config.APIURL,
		StoreId:              config.StoreID,
		AuthorizationModelId: config.AuthorizationModelID, // Optional: uses latest if empty
	}

	client, err := openfgaclient.NewSdkClient(configuration)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenFGA client: %w", err)
	}

	log.Printf("OpenFGA client initialized: URL=%s, StoreID=%s, ModelID=%s",
		config.APIURL, config.StoreID, config.AuthorizationModelID)

	return &OpenFGAClient{
		client: client,
		config: config,
	}, nil
}

// CheckParams holds parameters for an authorization check
type CheckParams struct {
	User     string // e.g., "user:alice"
	Relation string // e.g., "can_access"
	Object   string // e.g., "model:gpt-4"
}

// Check performs an authorization check against OpenFGA
// Returns true if the user has the specified relation to the object
func (c *OpenFGAClient) Check(ctx context.Context, params CheckParams) (bool, error) {
	log.Printf("OpenFGA Check: user=%s, relation=%s, object=%s",
		params.User, params.Relation, params.Object)

	// Build the check request
	body := openfgaclient.ClientCheckRequest{
		User:     params.User,
		Relation: params.Relation,
		Object:   params.Object,
	}

	// Perform the check
	checkRequest := c.client.Check(ctx).Body(body)
	data, err := c.client.CheckExecute(checkRequest)
	if err != nil {
		return false, fmt.Errorf("OpenFGA check failed: %w", err)
	}

	allowed := false
	if data.Allowed != nil {
		allowed = *data.Allowed
	}

	log.Printf("OpenFGA Check result: allowed=%v", allowed)

	return allowed, nil
}

// BatchCheck performs multiple authorization checks in a single call
// This can be used in the future for optimizing multiple resource checks
func (c *OpenFGAClient) BatchCheck(ctx context.Context, checks []CheckParams) ([]bool, error) {
	results := make([]bool, len(checks))

	// Note: OpenFGA SDK doesn't have native batch check, so we do individual checks
	// In a production system, you might want to parallelize these
	for i, check := range checks {
		allowed, err := c.Check(ctx, check)
		if err != nil {
			return nil, fmt.Errorf("batch check failed at index %d: %w", i, err)
		}
		results[i] = allowed
	}

	return results, nil
}

// ListObjects returns all objects of a given type that a user has a relation to
// This can be useful for filtering or listing accessible resources
func (c *OpenFGAClient) ListObjects(ctx context.Context, user, relation, objectType string) ([]string, error) {
	body := openfgaclient.ClientListObjectsRequest{
		User:     user,
		Relation: relation,
		Type:     objectType,
	}

	listRequest := c.client.ListObjects(ctx).Body(body)
	data, err := c.client.ListObjectsExecute(listRequest)
	if err != nil {
		return nil, fmt.Errorf("OpenFGA list objects failed: %w", err)
	}

	return data.Objects, nil
}

// ReadAuthorizationModel retrieves the current authorization model
// Useful for debugging and understanding the model structure
func (c *OpenFGAClient) ReadAuthorizationModel(ctx context.Context) (*openfga.AuthorizationModel, error) {
	readRequest := c.client.ReadAuthorizationModel(ctx)
	data, err := c.client.ReadAuthorizationModelExecute(readRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to read authorization model: %w", err)
	}

	return data.AuthorizationModel, nil
}
