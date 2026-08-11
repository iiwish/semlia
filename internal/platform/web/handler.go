package web

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed static
var distribution embed.FS

type handler struct {
	api        http.Handler
	assets     fs.FS
	fileServer http.Handler
}

func NewHandler(api http.Handler) http.Handler {
	if api == nil {
		panic("API handler is required")
	}
	assets, err := fs.Sub(distribution, "static")
	if err != nil {
		panic("embedded Web distribution is unavailable")
	}
	return &handler{
		api:        api,
		assets:     assets,
		fileServer: http.FileServer(http.FS(assets)),
	}
}

func (handler *handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if isAPIPath(request.URL.Path) {
		handler.api.ServeHTTP(response, request)
		return
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		response.Header().Set("Allow", "GET, HEAD")
		http.Error(response, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
	if name != "." && name != "" {
		if info, err := fs.Stat(handler.assets, name); err == nil && !info.IsDir() {
			response.Header().Set("X-Content-Type-Options", "nosniff")
			if strings.HasPrefix(name, "assets/") {
				response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				response.Header().Set("Cache-Control", "no-cache")
			}
			handler.fileServer.ServeHTTP(response, request)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			http.NotFound(response, request)
			return
		}
	}

	index, err := fs.ReadFile(handler.assets, "index.html")
	if err != nil {
		http.Error(response, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(response, request, "index.html", time.Time{}, bytes.NewReader(index))
}

func isAPIPath(value string) bool {
	return value == "/api" || strings.HasPrefix(value, "/api/") || value == "/health" || strings.HasPrefix(value, "/health/")
}
