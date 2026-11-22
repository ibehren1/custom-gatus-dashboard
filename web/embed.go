// Package web holds the embedded dashboard UI assets.
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var files embed.FS

// Static is the dashboard asset tree rooted at the static directory.
var Static, _ = fs.Sub(files, "static")
