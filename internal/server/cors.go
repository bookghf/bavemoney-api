package server

import (
	"net/http"
	"strings"
)

const (
	corsAllowedMethods = "GET, POST, PATCH, PUT, DELETE, OPTIONS"
	corsAllowedHeaders = "Authorization, Content-Type"
	corsMaxAge         = "600"
)

// cors lets browser clients (e.g. Expo web) call the API from the given
// origins. A "*" entry allows any origin. Preflight requests are answered here
// with 204 and never reach the router.
func cors(allowedOrigins []string, next http.Handler) http.Handler {
	allowAny := false
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if origin == "*" {
			allowAny = true
		}
		allowed[origin] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (allowAny || allowed[origin]) {
			header := w.Header()
			// Echo the origin rather than "*" so the response stays valid if
			// credentials are ever sent.
			header.Set("Access-Control-Allow-Origin", origin)
			header.Add("Vary", "Origin")
			header.Set("Access-Control-Allow-Methods", corsAllowedMethods)
			header.Set("Access-Control-Allow-Headers", corsAllowedHeaders)
			header.Set("Access-Control-Max-Age", corsMaxAge)
		}

		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// splitOrigins parses a comma-separated origin list, dropping blanks.
func splitOrigins(list string) []string {
	var origins []string
	for _, origin := range strings.Split(list, ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}
