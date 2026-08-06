//go:build webdist

package console

import "embed"

// Dist contains the built bundled console UI.
//
// all: includes Vite route chunks whose filenames begin with an underscore.
//
//go:embed all:dist
var Dist embed.FS

// DistRoot is the root directory inside Dist.
const DistRoot = "dist"
