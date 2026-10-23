// Package migrations embeds the SQL migrations applied by cmd/migrate.
package migrations

import "embed"

// FS holds every goose migration file in this directory.
//
//go:embed *.sql
var FS embed.FS
