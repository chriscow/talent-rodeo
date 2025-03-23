package assets

import (
	"embed"
)

//go:embed "emails" "migrations" "templates" "static" "prompts" "test-data"
var EmbeddedFiles embed.FS
