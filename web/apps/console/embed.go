//go:build !webdist

package console

import "embed"

// Dist contains a fallback page for source builds that have not built web/apps/console/dist.
//
//go:embed fallback
var Dist embed.FS

// DistRoot is the root directory inside Dist.
const DistRoot = "fallback"
