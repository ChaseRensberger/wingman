package web

import "embed"

// Dist contains the built bundled web UI.
//
//go:embed dist
var Dist embed.FS
