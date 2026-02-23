package templates

import "embed"

//go:embed *.tmpl *.md
var FS embed.FS
