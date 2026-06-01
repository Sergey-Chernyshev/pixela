package core

import "github.com/nrednav/cuid2"

// NewID returns a collision-resistant, URL-safe identifier for application-generated primary keys
// (projects, builds, snapshots, etc.). It mirrors Prisma's @default(cuid()) — IDs are cuid2 TEXT, not
// uuid/serial (data model contract; docs/architecture/go-backend.md §8.3). Image PKs are the sha256
// of the content, not this.
func NewID() string {
	return cuid2.Generate()
}
