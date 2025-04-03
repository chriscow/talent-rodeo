package assets

import (
	"embed"
)

//go:embed "emails" "migrations" "templates" "static" "prompts"
var EmbeddedFiles embed.FS
