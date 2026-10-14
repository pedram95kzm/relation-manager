package migrations

import "embed"

// Files embeds immutable, ordered SQL migrations in every server binary.
//
//go:embed *.sql
var Files embed.FS
