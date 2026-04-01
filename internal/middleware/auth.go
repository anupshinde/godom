package middleware

import "net/http"

// AuthFunc decides if a request is authorized.
type AuthFunc func(w http.ResponseWriter, r *http.Request) bool

// TokenAuth returns an AuthFunc that validates requests using a token.
// On first visit with ?token=... in the URL, sets a cookie. Subsequent
// requests are authenticated via the cookie.
func TokenAuth(token string) AuthFunc {
	return func(w http.ResponseWriter, r *http.Request) bool {
		if c, err := r.Cookie("godom_token"); err == nil && c.Value == token {
			return true
		}
		if r.URL.Query().Get("token") == token {
			http.SetCookie(w, &http.Cookie{
				Name:     "godom_token",
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			return true
		}
		return false
	}
}

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
