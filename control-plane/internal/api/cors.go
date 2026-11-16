package api

import (
	"net/http"
	"os"
	"strings"
)

func defaultAllowedOrigins() []string {
	return []string{
		"http://127.0.0.1:5173",
		"http://localhost:5173",
		"http://127.0.0.1:4173",
		"http://localhost:4173",
	}
}

func corsWildcardAllowed() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("NORIX_CORS_ALLOW_WILDCARD")))
	return v == "1" || v == "true" || v == "yes"
}

func appendOrigin(list []string, origin string) []string {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	if origin == "" {
		return list
	}
	return append(list, origin)
}

func allowedOrigins() (map[string]struct{}, bool) {
	origins := defaultAllowedOrigins()
	origins = appendOrigin(origins, os.Getenv("NORIX_WEB_ORIGIN"))
	origins = appendOrigin(origins, os.Getenv("NORIX_PUBLIC_URL"))
	allowAny := false
	if extra := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS")); extra != "" {
		for _, origin := range strings.Split(extra, ",") {
			origin = strings.TrimSpace(origin)
			if origin == "" {
				continue
			}
			if origin == "*" {
				if corsWildcardAllowed() {
					allowAny = true
				}
				continue
			}
			origins = appendOrigin(origins, origin)
		}
	}
	set := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		set[origin] = struct{}{}
	}
	return set, allowAny
}

// WithCORS allows browser calls from the Vite dev server, the panel origin,
// and optional extras. A CORS_ALLOWED_ORIGINS=* wildcard only takes effect
// when NORIX_CORS_ALLOW_WILDCARD=1.
func WithCORS(next http.Handler) http.Handler {
	allowed, allowAny := allowedOrigins()
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		_, listed := allowed[origin]
		if origin != "" && (allowAny || listed) {
			response.Header().Set("Access-Control-Allow-Origin", origin)
			response.Header().Set("Vary", "Origin")
			response.Header().Set("Access-Control-Allow-Credentials", "true")
			response.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Connect-Protocol-Version, Connect-Timeout-Ms, Grpc-Timeout, X-Grpc-Web, X-User-Agent")
			response.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			response.Header().Set("Access-Control-Expose-Headers", "Connect-Protocol-Version, Grpc-Status, Grpc-Message")
		}
		if request.Method == http.MethodOptions {
			response.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(response, request)
	})
}
