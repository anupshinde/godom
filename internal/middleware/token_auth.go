package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
)

// TokenAuth returns an AuthFunc that validates requests using a token.
// On first visit with ?token=... in the URL, sets a cookie. Subsequent
// requests are authenticated via the cookie.
func TokenAuth() (string, AuthFunc) {
	token := generateToken()
	return token, func(w http.ResponseWriter, r *http.Request) bool {
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

func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("godom: failed to generate auth token: %v", err)
	}
	return hex.EncodeToString(b)
}
