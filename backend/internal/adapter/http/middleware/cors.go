package middleware

import "net/http"

// CORS allows this REST API to be called from a browser-based frontend on a
// different origin (e.g. the agentsip admin UI running on its own port/host
// during development, or served from a different domain in production).
// Every route here is authenticated by an Authorization: Bearer header, not
// a cookie, so there is nothing for a third-party page to ride along on —
// reflecting the request's Origin (or allowing a configured allowlist) does
// not expose this API to cross-site request forgery the way a cookie-backed
// API would. allowedOrigins empty means "allow any origin" (the common case
// for a public API with no cookie-based session); non-empty restricts to
// exactly those origins.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if len(allowed) == 0 {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				} else if _, ok := allowed[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
				w.Header().Set("Vary", "Origin")
			}

			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
