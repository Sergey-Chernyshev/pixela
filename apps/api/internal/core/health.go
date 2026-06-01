package core

import "context"

// HealthChecker is a readiness probe for one dependency (Postgres, Redis, object store).
// Adapters implement it; httpapi aggregates a set of them for GET /readyz. Kept tiny on purpose
// (accept-interfaces-return-structs): the consumer defines exactly the methods it uses.
type HealthChecker interface {
	// Name identifies the dependency in the /readyz payload, e.g. "database", "redis", "objectstore".
	Name() string
	// Check performs a real, bounded round-trip; a non-nil error means the dependency is down.
	Check(ctx context.Context) error
}
