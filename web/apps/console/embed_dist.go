//go:build webdist

package console

import "embed"

// Dist contains the built bundled console UI.
//
//go:embed dist
var Dist embed.FS

// DistRoot is the root directory inside Dist.
const DistRoot = "dist"
