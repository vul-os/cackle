//go:build !embed_frontend

package main

import (
	"errors"
	"io/fs"
)

// Without the embed_frontend tag (plain `go build ./cmd/cackle`, or `go
// run` during backend-only iteration), there is no built SPA to serve.
// httpapi.Deps.WebFS is nil in this case, and internal/httpapi's SPA
// handler serves a clear "frontend not built" notice at "/" instead of a
// panic or a silent empty response — see docs/GETTING-STARTED.md.
func embeddedWebFS() (fs.FS, error) {
	return nil, errors.New("built without the embed_frontend tag; no frontend embedded (see docs/GETTING-STARTED.md)")
}
