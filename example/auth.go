package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hanh/ginertia"
)

// session is a minimal signed-cookie login, enough to show auth with
// Inertia. Use your real session store / JWT / OAuth in production —
// the Inertia side stays the same: protect routes in Go, redirect to /login.
type session struct{ secret []byte }

const sessionCookie = "demo_session"

func (s *session) sign(v string) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(v))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (s *session) user(c *gin.Context) (string, bool) {
	raw, err := c.Cookie(sessionCookie)
	if err != nil {
		return "", false
	}
	email, sig, ok := strings.Cut(raw, "|")
	if !ok || !hmac.Equal([]byte(sig), []byte(s.sign(email))) {
		return "", false
	}
	return email, true
}

func (s *session) login(c *gin.Context, email string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, email+"|"+s.sign(email), 7*24*3600, "/", "", false, true)
}

func (s *session) logout(c *gin.Context) {
	c.SetCookie(sessionCookie, "", -1, "/", "", false, true)
}

// shareUser exposes the logged-in user to every page as auth.user.
func (s *session) shareUser(c *gin.Context) {
	if email, ok := s.user(c); ok {
		name, _, _ := strings.Cut(email, "@")
		ginertia.Share(c, "auth", gin.H{"user": gin.H{"email": email, "name": name}})
	} else {
		ginertia.Share(c, "auth", gin.H{"user": nil})
	}
	c.Next()
}

// requireAuth sends guests to /login. Works for full page loads and for
// Inertia visits alike (the client follows the redirect).
func (s *session) requireAuth(c *gin.Context) {
	if _, ok := s.user(c); !ok {
		ginertia.Redirect(c, "/login")
		return
	}
	// Encrypt this user's page data in browser history so pressing Back
	// after logout cannot show private pages.
	ginertia.EncryptHistory(c, true)
	c.Next()
}

type LoginForm struct {
	Email    string `json:"email" form:"email" binding:"required,email"`
	Password string `json:"password" form:"password" binding:"required"`
}

func (h *handlers) loginForm(c *gin.Context) {
	if _, ok := h.sess.user(c); ok {
		ginertia.Redirect(c, "/")
		return
	}
	ginertia.Render(c, "Auth/Login", nil)
}

func (h *handlers) login(c *gin.Context) {
	var in LoginForm
	if err := c.ShouldBind(&in); err != nil {
		ginertia.WithErrors(c, ginertia.ValidationErrors(err, in))
		ginertia.Back(c)
		return
	}
	if in.Password != "secret" {
		ginertia.WithErrors(c, map[string]string{"email": "These credentials do not match our records."})
		ginertia.Back(c)
		return
	}
	h.sess.login(c, in.Email)
	ginertia.Flash(c, "success", "Welcome back!")
	ginertia.Redirect(c, "/")
}

func (h *handlers) logout(c *gin.Context) {
	h.sess.logout(c)
	ginertia.ClearHistory(c)
	ginertia.Redirect(c, "/login")
}
