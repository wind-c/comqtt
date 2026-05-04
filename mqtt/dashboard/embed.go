package dashboard

import "embed"

//go:embed all:templates all:static
var assetsFS embed.FS
