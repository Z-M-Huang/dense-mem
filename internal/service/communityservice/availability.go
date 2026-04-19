package communityservice

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// GraphNamePrefix is the prefix applied to all Dense-Mem GDS projected graphs.
// Orphan sweep uses this prefix to scope deletion to Dense-Mem–managed graphs.
//
// For a profile-scoped sweep, callers should append the profileID followed by
// a separator, e.g. GraphNamePrefix + profileID + "-", so that graphs
// belonging to other profiles are not touched.
const GraphNamePrefix = "dense-mem-"

// gdsClient is the minimal interface for running GDS read and write
// transactions against Neo4j. It is satisfied by the *Neo4jClient returned by
// storage/neo4j.NewClient — callers inject that value.
type gdsClient interface {
	ExecuteRead(ctx context.Context, fn neo4j.ManagedTransactionWork) (any, error)
	ExecuteWrite(ctx context.Context, fn neo4j.ManagedTransactionWork) (any, error)
}

// gdsQuerier abstracts the individual GDS operations needed by this package.
//
// This intermediate interface exists because neo4j.ManagedTransaction contains
// an unexported legacy() method, which makes it impossible to implement in
// test packages. By narrowing the surface to named operations, unit tests can
// provide simple stubs without a real Neo4j connection.
type gdsQuerier interface {
	// ProbeAvailability calls a lightweight GDS procedure. Returns nil when
	// GDS is installed and reachable; a non-nil error otherwise.
	ProbeAvailability(ctx context.Context) error

	// ListProjectedGraphs returns the names of all GDS projected graphs whose
	// names start with prefix.
	ListProjectedGraphs(ctx context.Context, prefix string) ([]string, error)

	// DropProjectedGraph drops the named GDS projected graph.
	// Implementations MUST use failIfMissing=false so concurrent callers do
	// not race on deletion.
	DropProjectedGraph(ctx context.Context, name string) error
}

// AvailabilityService exposes GDS health probing and orphan graph cleanup.
// All methods are safe for concurrent use.
type AvailabilityService interface {
	// ProbeGDS calls a lightweight GDS procedure to verify the plugin is
	// installed. Returns true when GDS is reachable; false otherwise.
	//
	// The result is intentionally non-fatal: callers should log the outcome
	// and continue startup without community detection when false is returned.
	ProbeGDS(ctx context.Context) bool

	// SweepOrphanGraphs lists and drops all GDS projected graphs whose names
	// start with prefix.
	//
	// Profile isolation: callers MUST pass a profile-scoped prefix
	// (e.g. GraphNamePrefix + profileID + "-") to avoid sweeping graphs that
	// belong to other profiles. Administrative cleanup may use GraphNamePrefix.
	SweepOrphanGraphs(ctx context.Context, prefix string) error
}

// availabilityServiceImpl is the production implementation of AvailabilityService.
type availabilityServiceImpl struct {
	querier gdsQuerier
	logger  *slog.Logger
}

// Compile-time check that availabilityServiceImpl satisfies AvailabilityService.
var _ AvailabilityService = (*availabilityServiceImpl)(nil)

// NewAvailabilityService constructs a ready-to-use AvailabilityService backed
// by client. logger may be nil; absent loggers emit no structured log lines.
func NewAvailabilityService(client gdsClient, logger *slog.Logger) AvailabilityService {
	return &availabilityServiceImpl{
		querier: &neo4jGDSQuerier{client: client},
		logger:  logger,
	}
}

// ProbeGDS calls gds.list() to verify GDS procedures are reachable.
// Any error is swallowed and logged; the method always returns a bool so that
// a missing GDS plugin never prevents application startup.
func (s *availabilityServiceImpl) ProbeGDS(ctx context.Context) bool {
	if err := s.querier.ProbeAvailability(ctx); err != nil {
		if s.logger != nil {
			s.logger.Warn("GDS availability probe failed — community detection disabled",
				slog.String("error", err.Error()),
			)
		}
		return false
	}
	return true
}

// SweepOrphanGraphs lists all GDS projected graphs starting with prefix and
// drops each one. Returns the first drop error encountered; earlier successful
// drops are not rolled back.
func (s *availabilityServiceImpl) SweepOrphanGraphs(ctx context.Context, prefix string) error {
	names, err := s.querier.ListProjectedGraphs(ctx, prefix)
	if err != nil {
		return fmt.Errorf("sweep orphan graphs list (prefix=%q): %w", prefix, err)
	}

	for _, name := range names {
		if err := s.querier.DropProjectedGraph(ctx, name); err != nil {
			return fmt.Errorf("sweep orphan graphs drop %q: %w", name, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// neo4jGDSQuerier — production gdsQuerier backed by a gdsClient
// ---------------------------------------------------------------------------

// neo4jGDSQuerier implements gdsQuerier using a real Neo4j client.
type neo4jGDSQuerier struct {
	client gdsClient
}

// ProbeAvailability runs CALL gds.list() in a read transaction. The procedure
// succeeds when GDS is installed and fails with a ClientError otherwise.
func (q *neo4jGDSQuerier) ProbeAvailability(ctx context.Context) error {
	_, err := q.client.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// gds.list() is the lightest GDS procedure available; it returns an
		// empty result set when no graphs are projected and raises a
		// ClientError when GDS is not installed.
		result, runErr := tx.Run(ctx,
			"CALL gds.list() YIELD graphName RETURN count(*) AS cnt LIMIT 1",
			nil,
		)
		if runErr != nil {
			return nil, runErr
		}
		_, consumeErr := result.Consume(ctx)
		return nil, consumeErr
	})
	return err
}

// ListProjectedGraphs returns graph names from gds.graph.list() filtered by
// prefix using a parameterised STARTS WITH clause to prevent injection.
func (q *neo4jGDSQuerier) ListProjectedGraphs(ctx context.Context, prefix string) ([]string, error) {
	raw, err := q.client.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, runErr := tx.Run(ctx,
			// Parameterised STARTS WITH prevents injection via prefix.
			"CALL gds.graph.list() YIELD graphName WHERE graphName STARTS WITH $prefix RETURN graphName",
			map[string]any{"prefix": prefix},
		)
		if runErr != nil {
			return nil, runErr
		}

		var names []string
		for result.Next(ctx) {
			v, ok := result.Record().Get("graphName")
			if !ok {
				continue
			}
			name, ok := v.(string)
			if !ok {
				continue
			}
			names = append(names, name)
		}
		if err := result.Err(); err != nil {
			return nil, err
		}
		return names, nil
	})
	if err != nil {
		return nil, err
	}
	names, _ := raw.([]string)
	return names, nil
}

// DropProjectedGraph calls gds.graph.drop($name, false). The second argument
// (failIfMissing=false) makes concurrent drops harmless.
func (q *neo4jGDSQuerier) DropProjectedGraph(ctx context.Context, name string) error {
	_, err := q.client.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, runErr := tx.Run(ctx,
			"CALL gds.graph.drop($name, false) YIELD graphName RETURN graphName",
			map[string]any{"name": name},
		)
		if runErr != nil {
			return nil, runErr
		}
		_, consumeErr := result.Consume(ctx)
		return nil, consumeErr
	})
	return err
}
