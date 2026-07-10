// Package web embeds the built frontend so the server ships as a single binary.
package web

import (
	"embed"
	"io/fs"
)

// dist holds the vite build output. The `all:` prefix keeps files that start
// with `.` or `_`. A committed dist/.gitkeep ensures this compiles even before
// the frontend is built (dev runs vite separately via `task dev`).
//
//go:embed all:dist
var dist embed.FS

// Dist returns the embedded frontend rooted at the dist directory.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic("web: embed dist subtree: " + err.Error())
	}
	return sub
}
