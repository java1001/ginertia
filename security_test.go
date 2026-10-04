package ginertia

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBackOnlyFollowsSameHostReferer(t *testing.T) {
	r := newRouter(newEngine(t))
	r.POST("/f", func(c *gin.Context) { Back(c, "/safe") })
	cases := map[string]string{
		"https://evil.example/phish":        "/safe",
		"//evil.example/x":                  "/safe",
		"http://example.com//evil.example/": "/safe",
		`http://example.com/\evil.example`:  "/%5Cevil.example", // escaped, stays on host
		"javascript:alert(1)":               "/safe",
		"http://EXAMPLE.com/users?page=2":   "/users?page=2",
		"":                                  "/safe",
	}
	for ref, want := range cases {
		w := do(r, "POST", "/f", inertia, hdr("Referer", ref))
		if got := w.Header().Get("Location"); got != want {
			t.Errorf("Referer %q → %q, want %q", ref, got, want)
		}
	}
}

func TestVersionMismatchIgnoresHostHeader(t *testing.T) {
	r := newRouter(newEngine(t))
	r.GET("/x", func(c *gin.Context) { Render(c, "X", nil) })
	w := do(r, "GET", "/x?a=1", inertia, hdr(HeaderVersion, "old"), func(r *http.Request) { r.Host = "evil.example" })
	if loc := w.Header().Get(HeaderLocation); loc != "/x?a=1" {
		t.Fatalf("host injection: %q", loc)
	}
}

// A request for "//evil.example/x" must not be echoed back as a
// protocol-relative URL (409 X-Inertia-Location or page.url): the client
// assigns those to window.location / history, which would leave the site.
func TestRequestURIIsNeverProtocolRelative(t *testing.T) {
	e := newEngine(t)
	r := newRouter(e)
	r.NoRoute(func(c *gin.Context) { Render(c, "NotFound", nil) })
	raw := func(path string) reqOpt {
		return func(r *http.Request) { r.URL = &url.URL{Path: path, RawQuery: "a=1"}; r.RequestURI = path + "?a=1" }
	}
	for _, p := range []string{"//evil.example/x", "///evil.example/x"} {
		w := do(r, "GET", "/", inertia, hdr(HeaderVersion, "old"), raw(p))
		if loc := w.Header().Get(HeaderLocation); w.Code != 409 || loc != "/evil.example/x?a=1" {
			t.Errorf("409 for %q: code %d, location %q", p, w.Code, loc)
		}
		if got := decodePage(t, do(r, "GET", "/", inertia, raw(p))).URL; got != "/evil.example/x?a=1" {
			t.Errorf("page.url for %q = %q", p, got)
		}
	}
}

// noEscapeMarshal mimics encoders that don't escape HTML (sonic.Marshal,
// json.Encoder with SetEscapeHTML(false), ...).
func noEscapeMarshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func TestPageScriptEscapedWithCustomMarshaler(t *testing.T) {
	e := MustNew(Config{RootView: "app.html", RootFS: mapFS(rootView), Version: "v1", Marshal: noEscapeMarshal})
	r := newRouter(e)
	payload := `</script><script>alert(1)</script><!--`
	r.GET("/", func(c *gin.Context) { Render(c, "P", gin.H{"v": payload}) })
	body := do(r, "GET", "/").Body.String()
	if n := len(scriptClose.FindAllStringIndex(body, -1)); n != 1 {
		t.Fatalf("script element closed early (%d closers): %s", n, body)
	}
	start := strings.Index(body, `application/json">`) + len(`application/json">`)
	end := strings.Index(body, "</script>")
	var p Page
	if err := json.Unmarshal([]byte(body[start:end]), &p); err != nil {
		t.Fatal(err)
	}
	if p.Props["v"] != payload {
		t.Fatalf("round trip: %q", p.Props["v"])
	}
}

func TestVaryIsAppended(t *testing.T) {
	e := newEngine(t)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Header("Vary", "Accept-Encoding"); c.Next() })
	r.Use(e.Middleware())
	r.GET("/", func(c *gin.Context) { Render(c, "P", nil) })
	vary := strings.Join(do(r, "GET", "/").Header().Values("Vary"), ",")
	if !strings.Contains(vary, "Accept-Encoding") || !strings.Contains(vary, HeaderInertia) {
		t.Fatalf("Vary = %q", vary)
	}
}

func TestFlashCannotBeForgedWithoutSecret(t *testing.T) {
	r := newRouter(newEngine(t)) // default CookieFlashStore, no Secret configured
	r.GET("/", func(c *gin.Context) { Render(c, "P", nil) })
	forged := base64.RawURLEncoding.EncodeToString([]byte(`{"e":{"x":"<b>forged</b>"},"f":{"success":"pwned"}}`))
	for _, v := range []string{forged, forged + ".AAAA", forged + "."} {
		p := decodePage(t, do(r, "GET", "/", inertia, func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: "ginertia_flash", Value: v})
		}))
		if len(p.Flash) != 0 || len(p.Props["errors"].(map[string]any)) != 0 {
			t.Fatalf("forged flash accepted: %q", v)
		}
	}
}

func TestLocationRejectsDangerousSchemes(t *testing.T) {
	r := newRouter(newEngine(t))
	var target string
	r.GET("/go", func(c *gin.Context) { Location(c, target) })
	for _, bad := range []string{"javascript:alert(1)", " JavaScript:alert(1)", "java\tscript:alert(1)",
		"\x00javascript:alert(1)", "data:text/html,<script>alert(1)</script>", "vbscript:msgbox(1)"} {
		target = bad
		w := do(r, "GET", "/go", inertia)
		if w.Code != 500 || w.Header().Get(HeaderLocation) != "" {
			t.Errorf("%q accepted (code %d)", bad, w.Code)
		}
	}
	for _, good := range []string{"https://github.com", "/dashboard", "http://localhost:3000/x"} {
		target = good
		if w := do(r, "GET", "/go", inertia); w.Code != 409 || w.Header().Get(HeaderLocation) != good {
			t.Errorf("%q rejected", good)
		}
	}
}

func TestValidationErrorsHidesParserErrors(t *testing.T) {
	var in struct{ A string }
	err := json.Unmarshal([]byte(`{bad`), &in)
	if got := ValidationErrors(err, in)["_"]; strings.Contains(got, "invalid character") {
		t.Fatalf("leaked parser error: %q", got)
	}
}

func TestCSRF(t *testing.T) {
	secret := []byte("s3cret")
	r := gin.New()
	r.Use(CSRF(CSRFConfig{Secret: secret, Skip: func(c *gin.Context) bool { return c.Request.URL.Path == "/hook" }}))
	r.GET("/", func(c *gin.Context) { c.String(200, "ok") })
	r.POST("/", func(c *gin.Context) { c.String(200, "ok") })
	r.POST("/hook", func(c *gin.Context) { c.String(200, "ok") })

	w := do(r, "GET", "/")
	var tok *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "XSRF-TOKEN" {
			tok = c
		}
	}
	if tok == nil || tok.HttpOnly {
		t.Fatal("token cookie must be set and readable by JS")
	}
	withCookie := func(v string) reqOpt {
		return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "XSRF-TOKEN", Value: v}) }
	}
	// Cookie alone (what a cross-site form gets) is not enough.
	if w := do(r, "POST", "/", withCookie(tok.Value)); w.Code != 419 {
		t.Fatalf("no header: %d", w.Code)
	}
	if w := do(r, "POST", "/", withCookie(tok.Value), hdr("X-XSRF-TOKEN", tok.Value)); w.Code != 200 {
		t.Fatalf("valid token rejected: %d", w.Code)
	}
	if w := do(r, "POST", "/", withCookie(tok.Value), hdr("X-XSRF-TOKEN", tok.Value+"x")); w.Code != 419 {
		t.Fatal("mismatched token accepted")
	}
	// Attacker-planted cookie (e.g. from a sibling subdomain) + matching header.
	fake := strings.Repeat("A", 43) + ".AAAA"
	if w := do(r, "POST", "/", withCookie(fake), hdr("X-XSRF-TOKEN", fake)); w.Code != 419 {
		t.Fatal("unsigned planted token accepted")
	}
	// Plain HTML form field.
	form := url.Values{"_token": {tok.Value}}.Encode()
	req := httptest.NewRequest("POST", "/", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "XSRF-TOKEN", Value: tok.Value})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("form field token rejected: %d", rec.Code)
	}
	if w := do(r, "POST", "/hook"); w.Code != 200 {
		t.Fatal("skip not honored")
	}
}

// A sibling subdomain can plant a validly signed XSRF-TOKEN cookie (cookie
// tossing) and send the matching _token / header. The token check alone
// accepts that; the Origin / Sec-Fetch-Site check must reject it.
func TestCSRFRejectsCrossOriginEvenWithValidToken(t *testing.T) {
	r := gin.New()
	r.Use(CSRF(CSRFConfig{Secret: []byte("s3cret"), TrustedOrigins: []string{"https://admin.example.com"}}))
	r.GET("/", func(c *gin.Context) { c.String(200, "ok") })
	r.POST("/", func(c *gin.Context) { c.String(200, "ok") })

	var tok string // attacker fetches a genuine signed token for themselves
	for _, ck := range do(r, "GET", "/").Result().Cookies() {
		if ck.Name == "XSRF-TOKEN" {
			tok = ck.Value
		}
	}
	post := func(opts ...reqOpt) int {
		base := []reqOpt{
			func(r *http.Request) {
				r.Host = "example.com"
				r.AddCookie(&http.Cookie{Name: "XSRF-TOKEN", Value: tok})
			},
			hdr("X-XSRF-TOKEN", tok),
		}
		return do(r, "POST", "/", append(base, opts...)...).Code
	}

	rejected := map[string][]reqOpt{
		"sibling subdomain (fetch metadata)": {hdr("Sec-Fetch-Site", "same-site"), hdr("Origin", "https://evil.example.com")},
		"cross-site (fetch metadata)":        {hdr("Sec-Fetch-Site", "cross-site"), hdr("Origin", "https://evil.test")},
		"sibling subdomain (Origin only)":    {hdr("Origin", "https://evil.example.com")},
		"cross-site (Origin only)":           {hdr("Origin", "https://evil.test")},
		"opaque origin":                      {hdr("Origin", "null")},
		"lookalike host":                     {hdr("Origin", "https://example.com.evil.test")},
	}
	for name, opts := range rejected {
		if code := post(opts...); code != 419 {
			t.Errorf("%s: got %d, want 419", name, code)
		}
	}
	allowed := map[string][]reqOpt{
		"same-origin (fetch metadata)":   {hdr("Sec-Fetch-Site", "same-origin"), hdr("Origin", "https://example.com")},
		"same-origin (Origin only)":      {hdr("Origin", "https://example.com")},
		"trusted origin":                 {hdr("Sec-Fetch-Site", "same-site"), hdr("Origin", "https://admin.example.com")},
		"non-browser client (no header)": nil,
	}
	for name, opts := range allowed {
		if code := post(opts...); code != 200 {
			t.Errorf("%s: got %d, want 200", name, code)
		}
	}
	// Same origin still needs the token.
	if code := do(r, "POST", "/", hdr("Sec-Fetch-Site", "same-origin")).Code; code != 419 {
		t.Errorf("same-origin without token: got %d", code)
	}
}

func TestCSRFRejectsInvalidTrustedOrigin(t *testing.T) {
	for _, bad := range []string{"example.com", "https://example.com/path", "*"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("TrustedOrigins %q accepted", bad)
				}
			}()
			CSRF(CSRFConfig{Secret: []byte("k"), TrustedOrigins: []string{bad}})
		}()
	}
}

var scriptClose = regexp.MustCompile(`(?i)</script`)

// Any prop value must round-trip through the HTML <script> payload intact
// and never close the script element early.
func FuzzPageScript(f *testing.F) {
	for _, s := range []string{"</script>", "<!--<script>", "</SCRIPT >", "  ", "<\\/script>", "]]>", "\x00"} {
		f.Add(s)
	}
	// Check both the default encoder and one that does not escape HTML.
	var val string
	var routers []*gin.Engine
	for _, m := range []func(any) ([]byte, error){nil, noEscapeMarshal} {
		r := newRouter(MustNew(Config{RootView: "app.html", RootFS: mapFS(rootView), Version: "v1", Marshal: m}))
		r.GET("/", func(c *gin.Context) { Render(c, "P", gin.H{"v": val}) })
		routers = append(routers, r)
	}
	f.Fuzz(func(t *testing.T, s string) {
		val = s
		for _, r := range routers {
			fuzzCheckPage(t, s, do(r, "GET", "/").Body.String())
		}
	})
}

func fuzzCheckPage(t *testing.T, s, body string) {
	t.Helper()
	start := strings.Index(body, `application/json">`) + len(`application/json">`)
	closers := scriptClose.FindAllStringIndex(body, -1)
	if len(closers) != 1 || closers[0][0] < start {
		t.Fatalf("script closed early for %q", s)
	}
	var p Page
	if err := json.Unmarshal([]byte(body[start:closers[0][0]]), &p); err != nil {
		t.Fatalf("bad json for %q: %v", s, err)
	}
	ref, _ := json.Marshal(s) // same invalid-UTF-8 replacement as the page
	var want string
	_ = json.Unmarshal(ref, &want)
	if p.Props["v"] != want {
		t.Fatalf("round trip %q → %q", s, p.Props["v"])
	}
}

func FuzzFlashCookie(f *testing.F) {
	f.Add("e30.x")
	f.Add("....")
	r := newRouter(newEngine(t0()))
	r.GET("/", func(c *gin.Context) { Render(c, "P", nil) })
	f.Fuzz(func(t *testing.T, v string) {
		w := do(r, "GET", "/", inertia, func(r *http.Request) { r.Header.Set("Cookie", "ginertia_flash="+v) })
		if w.Code != 200 {
			t.Fatalf("code %d for cookie %q", w.Code, v)
		}
	})
}
