package middleware

import "net/http"

// AuthFunc decides if a request is authorized.
type AuthFunc func(w http.ResponseWriter, r *http.Request) bool

// Wrap wraps an http.Handler with an AuthFunc. Requests that fail auth
// get a 401. Requests with ?token= in the query string are redirected
// to strip the token after the cookie is set.
func Wrap(next http.Handler, fn AuthFunc) http.Handler {
	if fn == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !fn(w, r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("token") != "" {
			http.Redirect(w, r, r.URL.Path, http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}
