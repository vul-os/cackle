package httpapi

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

// spaHandler serves the embedded React build at "/" with client-side-route
// fallback to index.html. It is mounted at the router's catch-all "/*",
// which chi only ever reaches for a path that didn't match anything under
// /api (a distinct, more specific branch of the routing tree) — so this
// can never shadow an API 404 with an HTML page.
func (s *server) spaHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.deps.WebFS == nil {
			writeError(w, http.StatusServiceUnavailable, codeFrontendMissing,
				"no embedded frontend in this binary; build with `make build` (see docs/GETTING-STARTED.md) or run the Vite dev server separately")
			return
		}

		upath := strings.TrimPrefix(r.URL.Path, "/")
		if upath == "" {
			upath = "index.html"
		}

		f, err := s.deps.WebFS.Open(upath)
		if err == nil {
			if stat, statErr := f.Stat(); statErr == nil && stat.IsDir() {
				_ = f.Close()
				f, err = s.deps.WebFS.Open("index.html")
				upath = "index.html"
			}
		}
		if err != nil {
			// Unknown static asset: fall back to index.html so client-side
			// routing (react-router et al.) can handle the path itself.
			f, err = s.deps.WebFS.Open("index.html")
			upath = "index.html"
			if err != nil {
				notFound(w, "not found")
				return
			}
		}
		defer f.Close()

		serveFileContent(w, r, upath, f)
	}
}

func serveFileContent(w http.ResponseWriter, r *http.Request, name string, f fs.File) {
	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(w, r, name, time.Time{}, rs)
		return
	}
	data, err := io.ReadAll(f)
	if err != nil {
		internalError(w, nil, "read embedded asset", err)
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}
