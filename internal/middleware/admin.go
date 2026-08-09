package middleware

import (
	"net/http"
	"strings"
)

func AdminAuth(apiKey string, notFoundBody []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				w.Write(notFoundBody)
				return
			}
			token := strings.TrimPrefix(auth, "Bearer ")
			if token != apiKey {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				w.Write(notFoundBody)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
