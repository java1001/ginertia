package ginertia

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// Redirect sends the client to another route of this app. It uses 303 for
// non-GET requests so the browser follows with GET (required by Inertia).
func Redirect(c *gin.Context, location string) {
	code := http.StatusFound
	if c.Request.Method != http.MethodGet {
		code = http.StatusSeeOther
	}
	c.Redirect(code, location)
	c.Abort()
}

// Back redirects to the previous page (Referer), or to fallback / "/".
// Only a same-host Referer is followed, reduced to path+query, so a request
// coming from another site can never bounce the user there (open redirect).
//
//	ginertia.WithErrors(c, errs)
//	ginertia.Back(c)
func Back(c *gin.Context, fallback ...string) {
	loc := "/"
	if len(fallback) > 0 && fallback[0] != "" {
		loc = fallback[0]
	}
	if ref, err := url.Parse(c.GetHeader("Referer")); err == nil {
		sameHost := ref.Host != "" && strings.EqualFold(ref.Host, c.Request.Host) &&
			(ref.Scheme == "http" || ref.Scheme == "https")
		pathOnly := ref.Host == "" && ref.Scheme == "" && strings.HasPrefix(ref.Path, "/")
		// "//evil.com" or "/\\evil.com" would be protocol-relative redirects.
		if p := ref.RequestURI(); (sameHost || pathOnly) &&
			!strings.HasPrefix(p, "//") && !strings.HasPrefix(p, "/\\") {
			loc = p
		}
	}
	Redirect(c, loc)
}

// Location forces a full page visit (window.location) — use it for external
// URLs or non-Inertia pages (OAuth, file downloads, another app).
// Only http(s) and relative URLs are allowed: the client assigns this to
// window.location, so a "javascript:" URL would be XSS.
//
// Location does NOT restrict the host ("//evil.example" is accepted, it is
// meant for external URLs). Never pass user input (?next=, ?redirect=)
// straight to it — check it with IsLocalURL first or keep an allowlist.
func Location(c *gin.Context, target string) {
	if u, err := url.Parse(strings.TrimSpace(target)); err != nil ||
		(u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https") {
		_ = c.Error(fmt.Errorf("ginertia: refusing unsafe Location %q", target))
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if IsInertia(c) {
		c.Header(HeaderLocation, target)
		c.AbortWithStatus(http.StatusConflict)
		return
	}
	c.Redirect(http.StatusFound, target)
	c.Abort()
}

// IsLocalURL reports whether target is a path on this site ("/dashboard",
// "/a?b=c") and not an absolute or protocol-relative URL ("https://x",
// "//x", "/\x"). Use it before redirecting to user-supplied URLs.
func IsLocalURL(target string) bool {
	if target == "" || target[0] != '/' {
		return false
	}
	if len(target) > 1 && (target[1] == '/' || target[1] == '\\') {
		return false
	}
	u, err := url.Parse(target)
	return err == nil && u.Scheme == "" && u.Host == ""
}
