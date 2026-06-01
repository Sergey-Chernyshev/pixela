// Package dbassets embeds the canonical SQL schema for application-time migration. The same
// schema.sql is the source of truth for sqlc (schema:) and Atlas (src:); this package only makes it
// available to the `pixela migrate` command without depending on the working directory.
package dbassets

import _ "embed"

// SchemaSQL is the full database schema (db/schema.sql).
//
//go:embed schema.sql
var SchemaSQL string
