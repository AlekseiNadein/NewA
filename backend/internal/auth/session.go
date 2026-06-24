package auth

import (
	"net/http"
	"strings"
)

const SessionCookieName = "nav_session"

const SessionCookieMaxAge = 12 * 60 * 60

func TokenFromRequest(r *http.Request) (string, error) {
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		if token := strings.TrimSpace(cookie.Value); token != "" {
			return token, nil
		}
	}

	return BearerToken(r.Header.Get("Authorization"))
}

func SetSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   SessionCookieMaxAge,
		HttpOnly: true,
		Secure:   requestIsSecure(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func ClearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   requestIsSecure(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func requestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}

	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
