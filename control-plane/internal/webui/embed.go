package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var dist embed.FS

func Handler() http.Handler {
	content, err := fs.Sub(dist, "dist")
	if err != nil {
		return http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			http.Error(response, "web ui unavailable", http.StatusServiceUnavailable)
		})
	}
	fileServer := http.FileServer(http.FS(content))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		path := strings.TrimPrefix(request.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if strings.Contains(path, "..") {
			http.Error(response, "invalid path", http.StatusBadRequest)
			return
		}
		if _, err := fs.Stat(content, path); err != nil {
			request.URL.Path = "/"
			setAssetHeaders(response, "index.html")
			fileServer.ServeHTTP(response, request)
			return
		}
		setAssetHeaders(response, path)
		fileServer.ServeHTTP(response, request)
	})
}

func setAssetHeaders(response http.ResponseWriter, path string) {
	switch {
	case strings.HasSuffix(path, ".html"):
		response.Header().Set("Cache-Control", "no-cache")
	case strings.HasPrefix(path, "assets/"):
		response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	default:
		response.Header().Set("Cache-Control", "public, max-age=300")
	}
}
