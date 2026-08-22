package dashboard

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const sessionCookieName = "sandrone_session"

type SessionStore struct {
	secret   []byte
	lifetime time.Duration
}

func NewSessionStore(secret string, lifetime time.Duration) *SessionStore {
	return &SessionStore{secret: []byte(secret), lifetime: lifetime}
}

func (s *SessionStore) Issue(writer http.ResponseWriter, username string) {
	expiry := time.Now().Add(s.lifetime).Unix()
	value := fmt.Sprintf("%s|%d", username, expiry)
	signed := value + "|" + s.sign(value)
	http.SetCookie(writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    base64.RawURLEncoding.EncodeToString([]byte(signed)),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(expiry, 0),
	})
}

func (s *SessionStore) Clear(writer http.ResponseWriter) {
	http.SetCookie(writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

func (s *SessionStore) Valid(request *http.Request) bool {
	cookie, err := request.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return false
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 {
		return false
	}
	value := parts[0] + "|" + parts[1]
	if subtle.ConstantTimeCompare([]byte(parts[2]), []byte(s.sign(value))) != 1 {
		return false
	}
	expiry, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix() < expiry
}

func (s *SessionStore) sign(value string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
