package httpx

import "embed"

var (
	//go:embed static
	staticFiles embed.FS
)
