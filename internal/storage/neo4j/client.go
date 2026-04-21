package neo4j

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	neo4jconfig "github.com/neo4j/neo4j-go-driver/v5/neo4j/config"
)

// Neo4jClientInterface is the companion interface for Neo4jClient.
// Consumers and tests depend on this abstraction rather than the concrete struct.
// This interface does NOT expose raw session.Run() to prevent bypassing read/write mode enforcement.
type Neo4jClientInterface interface {
	Verify(ctx context.Context) error
	ExecuteRead(ctx context.Context, fn neo4j.ManagedTransactionWork) (any, error)
	ExecuteWrite(ctx context.Context, fn neo4j.ManagedTransactionWork) (any, error)
	Close(ctx context.Context) error
}

// Neo4jClient wraps a Neo4j driver with proper session mode enforcement.
// Services must use ExecuteRead or ExecuteWrite to ensure proper cluster routing.
type Neo4jClient struct {
	driver   neo4j.DriverWithContext
	database string
}

// Ensure Neo4jClient implements Neo4jClientInterface
var _ Neo4jClientInterface = (*Neo4jClient)(nil)

// ConfigProvider defines the configuration needed for Neo4j connection.
type ConfigProvider interface {
	GetNeo4jURI() string
	GetNeo4jUser() string
	GetNeo4jPassword() string
	GetNeo4jDatabase() string
}

// NewClient creates a new Neo4j client with configured connection pool settings.
// It establishes the connection and verifies connectivity.
// Server startup must fail if connectivity verification fails.
func NewClient(ctx context.Context, cfg ConfigProvider) (*Neo4jClient, error) {
	uri := cfg.GetNeo4jURI()
	user := cfg.GetNeo4jUser()
	password := cfg.GetNeo4jPassword()
	database := cfg.GetNeo4jDatabase()

	if uri == "" {
		return nil, fmt.Errorf("neo4j URI is empty")
	}
	if user == "" {
		return nil, fmt.Errorf("neo4j user is empty")
	}
	if password == "" {
		return nil, fmt.Errorf("neo4j password is empty")
	}

	// Create driver with configured pool settings
	driver, err := neo4j.NewDriverWithContext(
		uri,
		neo4j.BasicAuth(user, password, ""),
		func(config *neo4jconfig.Config) {
			config.MaxConnectionPoolSize = 100
			config.ConnectionAcquisitionTimeout = 60 * time.Second
			config.MaxConnectionLifetime = 1 * time.Hour
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create neo4j driver: %w", err)
	}

	// Verify connectivity with context
	if err := driver.VerifyConnectivity(ctx); err != nil {
		_ = driver.Close(ctx)
		return nil, fmt.Errorf("failed to verify neo4j connectivity: %w", err)
	}

	// Default database to "neo4j" if not specified
	if database == "" {
		database = "neo4j"
	}

	return &Neo4jClient{
		driver:   driver,
		database: database,
	}, nil
}

// Verify executes a simple query to verify the database is accessible.
// This is used for health/readiness checks.
func (c *Neo4jClient) Verify(ctx context.Context) error {
	session := c.driver.NewSession(ctx, neo4j.SessionConfig{
		AccessMode:   neo4j.AccessModeRead,
		DatabaseName: c.database,
	})
	defer session.Close(ctx)

	_, err := session.Run(ctx, "RETURN 1", nil)
	return err
}

// ExecuteRead executes a read-only transaction in the appropriate session mode.
// This routes to read replicas in a cluster configuration.
func (c *Neo4jClient) ExecuteRead(ctx context.Context, fn neo4j.ManagedTransactionWork) (any, error) {
	session := c.driver.NewSession(ctx, neo4j.SessionConfig{
		AccessMode:   neo4j.AccessModeRead,
		DatabaseName: c.database,
	})
	defer session.Close(ctx)

	return session.ExecuteRead(ctx, fn)
}

// ExecuteWrite executes a write transaction in the appropriate session mode.
// This routes to the leader in a cluster configuration.
func (c *Neo4jClient) ExecuteWrite(ctx context.Context, fn neo4j.ManagedTransactionWork) (any, error) {
	session := c.driver.NewSession(ctx, neo4j.SessionConfig{
		AccessMode:   neo4j.AccessModeWrite,
		DatabaseName: c.database,
	})
	defer session.Close(ctx)

	return session.ExecuteWrite(ctx, fn)
}

// Close closes the Neo4j driver connection.
func (c *Neo4jClient) Close(ctx context.Context) error {
	return c.driver.Close(ctx)
}
