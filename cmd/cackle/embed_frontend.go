//go:build embed_frontend

package main

import (
	"embed"
	"io/fs"
)

// The built SPA is copied into ./dist immediately before compiling with
// this tag (see the Makefile's `build` target and the Dockerfile's "EMBED
// CONTRACT" comment) — go:embed cannot reach into a sibling directory
// (../web/dist), so the copy is what makes `all:dist` resolvable here.
//
//go:embed all:dist
var embeddedDistFS embed.FS

// embeddedWebFS returns the built frontend, rooted at its own static files
// (index.html etc), for httpapi.Deps.WebFS.
func embeddedWebFS() (fs.FS, error) {
	return fs.Sub(embeddedDistFS, "dist")
}
