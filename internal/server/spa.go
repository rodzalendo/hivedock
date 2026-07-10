package server

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
)

// SPAHandler serves the embedded frontend build. Unknown paths fall back to
// index.html so client-side routing works. If the frontend hasn't been built
// (dev builds run vite separately via `task dev`), it returns a clear hint
// instead of a bare 404.
func SPAHandler(dist fs.FS, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}

		if serveFile(w, r, dist, name) {
			return
		}
		// Fall back to the SPA entrypoint for client-side routes.
		if serveFile(w, r, dist, "index.html") {
			return
		}
		logger.Warn("spa: frontend not built", "path", r.URL.Path)
		http.Error(w, "frontend not built — run `task build` (or `cd web && npm run build`)", http.StatusNotFound)
	}
}

func serveFile(w http.ResponseWriter, r *http.Request, dist fs.FS, name string) bool {
	f, err := dist.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		return false
	}

	seeker, ok := f.(io.ReadSeeker)
	if !ok {
		// Embedded files implement Seeker; this is a defensive fallback.
		w.Header().Set("Content-Type", contentType(name))
		io.Copy(w, f)
		return true
	}
	http.ServeContent(w, r, name, stat.ModTime(), seeker)
	return true
}

func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}
