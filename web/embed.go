// Package web is floe's frontend: a Svelte app that Vite builds into dist/,
// embedded in the binary. Nothing is fetched at runtime.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist is the built frontend, rooted at dist/.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // "dist" is a valid path, which is all fs.Sub checks
	}
	return sub
}
