package auth

import (
	"net/http"
	"strings"
)

// Middleware enforces `Authorization: Bearer <psk>`.
// Health endpoint is left open so `peers`/`status` can probe liveness
// without knowing the secret; everything else requires the PSK.
func Middleware(psk string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			next.ServeHTTP(w, r)
			return
		}
		h := r.Header.Get("Authorization")
		want := "Bearer " + psk
		if h == "" {
			// also allow ?token= for simple curl debugging
			if r.URL.Query().Get("token") == psk {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		if subtleCompare(h, want) {
			next.ServeHTTP(w, r)
			return
		}
		_ = strings.TrimSpace(h)
		http.Error(w, "bad token", http.StatusUnauthorized)
	})
}

func subtleCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
