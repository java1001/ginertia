package ginertia

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// CSRFConfig configures the CSRF middleware.
type CSRFConfig struct {
	// Secret signs tokens. Required; use the same value on every instance.
	Secret []byte
	// CookieName / HeaderName default to what the Inertia client uses
	// automatically: it reads the XSRF-TOKEN cookie and sends X-XSRF-TOKEN.
	CookieName string
	HeaderName string
	// FormField is checked when the header is absent (plain HTML forms).
	FormField string // default "_token"
	// Secure marks the cookie Secure. Auto-enabled on TLS requests.
	Secure bool
	// TrustedOrigins are extra origins ("https://app.example.com", scheme +
	// host [+ port], no path) allowed to send unsafe requests from another
	// origin, e.g. a separate frontend domain, or when a reverse proxy
	// rewrites the Host header so it no longer matches the browser's Origin.
	TrustedOrigins []string
	// Skip lets requests through unchecked (e.g. webhooks with their own auth).
	Skip func(c *gin.Context) bool
	// OnFailure handles a rejected request. Default: 419 "Page Expired"
	// (same status as Laravel, handle it with router.on('httpException')).
	OnFailure func(c *gin.Context)
}

// CSRF protects POST/PUT/PATCH/DELETE with two independent checks:
//
//  1. Origin check: browsers label every request with Sec-Fetch-Site (or at
//     least Origin). Unsafe requests from another origin are rejected —
//     including sibling subdomains ("same-site"), which can plant cookies and
//     would otherwise defeat a double-submit token. Requests without either
//     header (non-browser clients) fall through to the token check.
//  2. Signed double-submit token: the XSRF-TOKEN cookie must be HMAC-signed
//     with Secret and match the X-XSRF-TOKEN header (or the _token field).
//
// No frontend code is needed: the Inertia client sends the header itself.
//
//	r.Use(ginertia.CSRF(ginertia.CSRFConfig{Secret: []byte(os.Getenv("APP_KEY"))}))
func CSRF(cfg CSRFConfig) gin.HandlerFunc {
	if len(cfg.Secret) == 0 {
		panic("ginertia: CSRFConfig.Secret is required")
	}
	if cfg.CookieName == "" {
		cfg.CookieName = "XSRF-TOKEN"
	}
	if cfg.HeaderName == "" {
		cfg.HeaderName = "X-XSRF-TOKEN"
	}
	if cfg.FormField == "" {
		cfg.FormField = "_token"
	}
	if cfg.OnFailure == nil {
		cfg.OnFailure = func(c *gin.Context) { c.AbortWithStatus(419) }
	}
	trusted := make(map[string]bool, len(cfg.TrustedOrigins))
	for _, o := range cfg.TrustedOrigins {
		u, err := url.Parse(o)
		if err != nil || u.Scheme == "" || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" {
			panic(fmt.Sprintf("ginertia: invalid CSRFConfig.TrustedOrigins entry %q (want scheme://host[:port])", o))
		}
		trusted[strings.ToLower(u.Scheme+"://"+u.Host)] = true
	}
	sign := func(nonce string) string {
		m := hmac.New(sha256.New, cfg.Secret)
		m.Write([]byte("ginertia-csrf:" + nonce))
		return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
	}
	valid := func(tok string) bool {
		nonce, sig, ok := strings.Cut(tok, ".")
		return ok && len(nonce) >= 32 && hmac.Equal([]byte(sig), []byte(sign(nonce)))
	}
	issue := func(c *gin.Context) string {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		nonce := base64.RawURLEncoding.EncodeToString(b)
		tok := nonce + "." + sign(nonce)
		http.SetCookie(c.Writer, &http.Cookie{
			Name: cfg.CookieName, Value: tok, Path: "/",
			// Readable by JS on purpose: the client copies it into the header.
			HttpOnly: false, SameSite: http.SameSiteLaxMode,
			Secure: cfg.Secure || c.Request.TLS != nil,
		})
		return tok
	}

	return func(c *gin.Context) {
		cookie, _ := c.Cookie(cfg.CookieName)
		if !valid(cookie) {
			cookie = ""
		}
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
			if cookie == "" {
				issue(c)
			}
			c.Next()
			return
		}
		if cfg.Skip != nil && cfg.Skip(c) {
			c.Next()
			return
		}
		if !sameOrigin(c.Request, trusted) {
			cfg.OnFailure(c)
			return
		}
		sent := c.GetHeader(cfg.HeaderName)
		if sent == "" {
			sent = c.PostForm(cfg.FormField)
		}
		if cookie == "" || !hmac.Equal([]byte(sent), []byte(cookie)) {
			if cookie == "" {
				issue(c) // let the next attempt succeed
			}
			cfg.OnFailure(c)
			return
		}
		c.Next()
	}
}

// sameOrigin reports whether an unsafe request comes from the app's own
// origin (or a trusted one). Same logic as Go 1.25's
// http.CrossOriginProtection:
//
//   - Sec-Fetch-Site "same-origin" or "none" (typed URL, bookmark) → allowed;
//     any other value ("same-site", "cross-site") → only a trusted Origin.
//   - No Sec-Fetch-Site (older browsers): no Origin → allowed (not a browser
//     request, the token check still applies); Origin host == Host → allowed;
//     otherwise only a trusted Origin.
func sameOrigin(r *http.Request, trusted map[string]bool) bool {
	origin := r.Header.Get("Origin")
	isTrusted := origin != "" && trusted[strings.ToLower(origin)]
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "":
		// fall through to the Origin check
	default:
		return isTrusted
	}
	if origin == "" {
		return true
	}
	if u, err := url.Parse(origin); err == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host) {
		return true
	}
	return isTrusted
}
