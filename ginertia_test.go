package ginertia

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

const rootView = `<!doctype html><html><head>{{ .inertiaHead }}<title>{{ .title }}</title></head><body>{{ .inertia }}</body></html>`

func newEngine(t *testing.T, mod ...func(*Config)) *Engine {
	t.Helper()
	cfg := Config{
		RootView: "app.html",
		RootFS:   fstest.MapFS{"app.html": {Data: []byte(rootView)}},
		Version:  "v1",
	}
	for _, m := range mod {
		m(&cfg)
	}
	return MustNew(cfg)
}

func newRouter(e *Engine) *gin.Engine {
	r := gin.New()
	r.Use(e.Middleware())
	return r
}

type reqOpt func(*http.Request)

func inertia(r *http.Request) {
	r.Header.Set(HeaderInertia, "true")
	r.Header.Set(HeaderVersion, "v1")
}

func hdr(k, v string) reqOpt { return func(r *http.Request) { r.Header.Set(k, v) } }

func do(r http.Handler, method, url string, opts ...reqOpt) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, nil)
	for _, o := range opts {
		o(req)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodePage(t *testing.T, w *httptest.ResponseRecorder) Page {
	t.Helper()
	var p Page
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode page: %v\nbody: %s", err, w.Body.String())
	}
	return p
}

func TestFirstVisitRendersHTML(t *testing.T) {
	e := newEngine(t)
	r := newRouter(e)
	r.GET("/users", func(c *gin.Context) {
		ViewData(c, "title", "Users")
		Render(c, "Users/Index", gin.H{"users": []string{"a", "</script><b>x"}})
	})
	w := do(r, "GET", "/users?page=2")

	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("got %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	body := w.Body.String()
	if !strings.Contains(body, `<script data-page="app" type="application/json">`) ||
		!strings.Contains(body, `<div id="app"></div>`) {
		t.Fatalf("missing inertia markup: %s", body)
	}
	if strings.Count(body, "</script>") != 1 {
		t.Fatalf("page JSON broke out of script tag: %s", body)
	}
	if !strings.Contains(body, "<title>Users</title>") {
		t.Fatal("view data not rendered")
	}
	if w.Header().Get("Vary") != HeaderInertia {
		t.Fatal("missing Vary header")
	}
	// Extract and parse the JSON payload.
	start := strings.Index(body, `application/json">`) + len(`application/json">`)
	end := strings.Index(body, "</script>")
	var p Page
	if err := json.Unmarshal([]byte(body[start:end]), &p); err != nil {
		t.Fatal(err)
	}
	if p.Component != "Users/Index" || p.URL != "/users?page=2" || p.Version != "v1" {
		t.Fatalf("bad page %+v", p)
	}
}

func TestInertiaVisitReturnsJSON(t *testing.T) {
	e := newEngine(t)
	e.Share("app", "demo")
	r := newRouter(e)
	r.Use(func(c *gin.Context) { Share(c, "auth", gin.H{"user": "hanh"}); c.Next() })
	r.GET("/", func(c *gin.Context) {
		Render(c, "Home", gin.H{"n": func() int { return 42 }})
	})
	w := do(r, "GET", "/", inertia)
	if w.Header().Get(HeaderInertia) != "true" {
		t.Fatal("missing X-Inertia response header")
	}
	p := decodePage(t, w)
	if p.Props["app"] != "demo" || p.Props["n"].(float64) != 42 {
		t.Fatalf("props %+v", p.Props)
	}
	if p.Props["auth"].(map[string]any)["user"] != "hanh" {
		t.Fatal("request share missing")
	}
	if _, ok := p.Props["errors"].(map[string]any); !ok {
		t.Fatal("errors prop must always be an object")
	}
	if strings.Join(p.SharedProps, ",") != "app,auth" {
		t.Fatalf("sharedProps %v", p.SharedProps)
	}
}

func TestVersionMismatch(t *testing.T) {
	r := newRouter(newEngine(t))
	r.GET("/x", func(c *gin.Context) { Render(c, "X", nil) })
	w := do(r, "GET", "/x?a=1", inertia, hdr(HeaderVersion, "old"))
	if w.Code != http.StatusConflict || w.Header().Get(HeaderLocation) != "/x?a=1" {
		t.Fatalf("got %d %q", w.Code, w.Header().Get(HeaderLocation))
	}
	// Non-GET requests are not version-checked.
	r.POST("/x", func(c *gin.Context) { Render(c, "X", nil) })
	if w := do(r, "POST", "/x", inertia, hdr(HeaderVersion, "old")); w.Code != 200 {
		t.Fatalf("POST got %d", w.Code)
	}
}

func TestPartialReload(t *testing.T) {
	r := newRouter(newEngine(t))
	calls := 0
	r.GET("/", func(c *gin.Context) {
		Render(c, "Dash", gin.H{
			"a":      1,
			"b":      func() any { calls++; return 2 },
			"opt":    Optional(func() any { return "o" }),
			"always": Always("x"),
		})
	})

	p := decodePage(t, do(r, "GET", "/", inertia))
	if _, ok := p.Props["opt"]; ok {
		t.Fatal("optional prop sent on normal visit")
	}

	calls = 0
	p = decodePage(t, do(r, "GET", "/", inertia,
		hdr(HeaderPartialComponent, "Dash"), hdr(HeaderPartialData, "a,opt")))
	if p.Props["a"] == nil || p.Props["opt"] != "o" || p.Props["always"] != "x" || p.Props["errors"] == nil {
		t.Fatalf("partial only: %+v", p.Props)
	}
	if _, ok := p.Props["b"]; ok || calls != 0 {
		t.Fatal("excluded prop was resolved")
	}

	p = decodePage(t, do(r, "GET", "/", inertia,
		hdr(HeaderPartialComponent, "Dash"), hdr(HeaderPartialExcept, "a")))
	if _, ok := p.Props["a"]; ok || p.Props["b"] == nil {
		t.Fatalf("partial except: %+v", p.Props)
	}

	// Partial headers for another component are ignored.
	p = decodePage(t, do(r, "GET", "/", inertia,
		hdr(HeaderPartialComponent, "Other"), hdr(HeaderPartialData, "a")))
	if p.Props["b"] == nil {
		t.Fatal("partial applied to wrong component")
	}
}

func TestDeferMergeOnce(t *testing.T) {
	r := newRouter(newEngine(t))
	r.GET("/", func(c *gin.Context) {
		Render(c, "Feed", gin.H{
			"stats":  Defer(func() any { return 1 }),
			"chart":  Defer(func() any { return 2 }, "slow"),
			"posts":  Merge([]int{3}).MatchOn("id"),
			"newest": Merge([]int{0}).Prepend(),
			"cfg":    DeepMerge(gin.H{"k": 1}),
			"plans":  Once(func() any { return "p" }).Until(time.Hour),
		})
	})

	p := decodePage(t, do(r, "GET", "/", inertia))
	if p.DeferredProps["default"][0] != "stats" || p.DeferredProps["slow"][0] != "chart" {
		t.Fatalf("deferredProps %+v", p.DeferredProps)
	}
	if _, ok := p.Props["stats"]; ok {
		t.Fatal("deferred prop sent on first load")
	}
	if strings.Join(p.MergeProps, ",") != "posts" || strings.Join(p.PrependProps, ",") != "newest" ||
		strings.Join(p.DeepMergeProps, ",") != "cfg" || strings.Join(p.MatchPropsOn, ",") != "posts.id" {
		t.Fatalf("merge meta %+v %+v %+v %+v", p.MergeProps, p.PrependProps, p.DeepMergeProps, p.MatchPropsOn)
	}
	if p.OnceProps["plans"].Prop != "plans" || p.OnceProps["plans"].ExpiresAt == nil || p.Props["plans"] != "p" {
		t.Fatalf("once %+v", p.OnceProps)
	}

	// Client loads the deferred group.
	p = decodePage(t, do(r, "GET", "/", inertia,
		hdr(HeaderPartialComponent, "Feed"), hdr(HeaderPartialData, "stats")))
	if p.Props["stats"].(float64) != 1 || len(p.DeferredProps) != 0 {
		t.Fatalf("deferred load %+v", p)
	}

	// Reset header disables merging; once props the client holds are skipped.
	p = decodePage(t, do(r, "GET", "/", inertia,
		hdr(HeaderReset, "posts"), hdr(HeaderExceptOnceProps, "plans")))
	if len(p.MergeProps) != 0 {
		t.Fatal("reset prop still merged")
	}
	if _, ok := p.Props["plans"]; ok || p.OnceProps["plans"].Prop == "" {
		t.Fatal("once prop resent or metadata dropped")
	}
}

func TestErrorsAndFlashSurviveRedirect(t *testing.T) {
	e := newEngine(t, func(c *Config) { c.Flash = &CookieFlashStore{Secret: []byte("k")} })
	r := newRouter(e)
	r.POST("/users", func(c *gin.Context) {
		WithErrors(c, map[string]string{"email": "required"})
		Flash(c, "error", "Check the form")
		Back(c)
	})
	r.GET("/users/create", func(c *gin.Context) { Render(c, "Users/Create", nil) })

	w := do(r, "POST", "/users", inertia, hdr("Referer", "/users/create"))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/users/create" {
		t.Fatalf("redirect %d %s", w.Code, w.Header().Get("Location"))
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("want exactly one flash cookie, got %d", len(cookies))
	}

	next := func(cs ...*http.Cookie) *httptest.ResponseRecorder {
		return do(r, "GET", "/users/create", inertia, func(req *http.Request) {
			for _, c := range cs {
				req.AddCookie(c)
			}
		})
	}
	w2 := next(cookies[0])
	p := decodePage(t, w2)
	if p.Props["errors"].(map[string]any)["email"] != "required" || p.Flash["error"] != "Check the form" {
		t.Fatalf("errors/flash not delivered: %+v %+v", p.Props["errors"], p.Flash)
	}
	// The cookie is cleared after being read.
	if c := w2.Result().Cookies(); len(c) != 1 || c[0].MaxAge >= 0 {
		t.Fatal("flash cookie not cleared")
	}

	// Tampered cookie is rejected.
	bad := *cookies[0]
	bad.Value = strings.Replace(bad.Value, ".", "x.", 1)
	if p := decodePage(t, next(&bad)); len(p.Props["errors"].(map[string]any)) != 0 {
		t.Fatal("tampered flash accepted")
	}
}

func TestErrorBag(t *testing.T) {
	r := newRouter(newEngine(t))
	r.POST("/", func(c *gin.Context) {
		WithErrors(c, map[string]string{"name": "x"})
		Render(c, "P", nil) // same-request render consumes the flash
	})
	w := do(r, "POST", "/", inertia, hdr(HeaderErrorBag, "createUser"))
	p := decodePage(t, w)
	if p.Props["errors"].(map[string]any)["createUser"].(map[string]any)["name"] != "x" {
		t.Fatalf("error bag %+v", p.Props["errors"])
	}
	if c := w.Result().Cookies(); len(c) != 1 || c[0].MaxAge >= 0 {
		t.Fatal("consumed flash should be cleared, not persisted")
	}
}

func TestRedirect302To303(t *testing.T) {
	r := newRouter(newEngine(t))
	for _, m := range []string{"PUT", "PATCH", "DELETE"} {
		r.Handle(m, "/u", func(c *gin.Context) { c.Redirect(http.StatusFound, "/u") })
		if w := do(r, m, "/u", inertia); w.Code != http.StatusSeeOther {
			t.Fatalf("%s: got %d", m, w.Code)
		}
	}
	r.POST("/p", func(c *gin.Context) { c.Redirect(http.StatusFound, "/u") })
	if w := do(r, "POST", "/p", inertia); w.Code != http.StatusFound {
		t.Fatalf("POST should keep 302, got %d", w.Code)
	}
}

func TestLocation(t *testing.T) {
	r := newRouter(newEngine(t))
	r.GET("/out", func(c *gin.Context) { Location(c, "https://github.com") })
	w := do(r, "GET", "/out", inertia)
	if w.Code != http.StatusConflict || w.Header().Get(HeaderLocation) != "https://github.com" {
		t.Fatalf("got %d", w.Code)
	}
	if w := do(r, "GET", "/out"); w.Code != http.StatusFound {
		t.Fatalf("non-inertia got %d", w.Code)
	}
}

func TestResolverError(t *testing.T) {
	r := newRouter(newEngine(t))
	r.GET("/", func(c *gin.Context) {
		Render(c, "P", gin.H{"x": func(*gin.Context) (any, error) { return nil, errors.New("db down") }})
	})
	if w := do(r, "GET", "/", inertia); w.Code != 500 {
		t.Fatalf("got %d", w.Code)
	}
}

func TestHistoryFlags(t *testing.T) {
	r := newRouter(newEngine(t, func(c *Config) { c.EncryptHistory = true }))
	r.GET("/", func(c *gin.Context) { Render(c, "P", nil) })
	r.GET("/plain", func(c *gin.Context) { EncryptHistory(c, false); Render(c, "P", nil) })
	r.POST("/logout", func(c *gin.Context) { ClearHistory(c); Redirect(c, "/") })

	if p := decodePage(t, do(r, "GET", "/", inertia)); !p.EncryptHistory || p.ClearHistory {
		t.Fatalf("%+v", p)
	}
	if p := decodePage(t, do(r, "GET", "/plain", inertia)); p.EncryptHistory {
		t.Fatal("per-request override ignored")
	}
	w := do(r, "POST", "/logout", inertia)
	p := decodePage(t, do(r, "GET", "/", inertia, func(req *http.Request) { req.AddCookie(w.Result().Cookies()[0]) }))
	if !p.ClearHistory {
		t.Fatal("clearHistory lost across redirect")
	}
}

func TestViteDevAndManifest(t *testing.T) {
	dir := t.TempDir()
	hot := filepath.Join(dir, "hot")
	os.WriteFile(hot, []byte("http://localhost:5173\n"), 0o644)
	v := &Vite{HotFile: hot}
	html, _ := v.Tags("src/app.js")
	if !strings.Contains(string(html), `src="http://localhost:5173/@vite/client"`) ||
		!strings.Contains(string(html), `src="http://localhost:5173/src/app.js"`) {
		t.Fatal(html)
	}

	os.Remove(hot)
	manifest := `{
	  "src/app.js": {"file":"assets/app-1.js","isEntry":true,"css":["assets/app-1.css"],"imports":["_shared.js"]},
	  "_shared.js": {"file":"assets/shared-2.js","css":["assets/shared-2.css"]}
	}`
	v = &Vite{HotFile: hot, FS: fstest.MapFS{"m.json": {Data: []byte(manifest)}}, ManifestPath: "m.json"}
	html, err := v.Tags("src/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<link rel="stylesheet" href="/build/assets/app-1.css">`,
		`<link rel="stylesheet" href="/build/assets/shared-2.css">`,
		`<link rel="modulepreload" href="/build/assets/shared-2.js">`,
		`<script type="module" src="/build/assets/app-1.js"></script>`,
	} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("missing %s in %s", want, html)
		}
	}

	e := MustNew(Config{RootView: "app.html", RootFS: fstest.MapFS{"app.html": {Data: []byte(rootView)}}, Vite: v})
	if len(e.Version()) != 32 {
		t.Fatalf("version from manifest: %q", e.Version())
	}
}

func TestSSR(t *testing.T) {
	ssr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p Page
		json.NewDecoder(r.Body).Decode(&p)
		json.NewEncoder(w).Encode(map[string]any{
			"head": []string{"<title>SSR " + p.Component + "</title>"},
			"body": `<div data-server-rendered="true" id="app"><h1>hi</h1></div>`,
		})
	}))
	defer ssr.Close()

	r := newRouter(newEngine(t, func(c *Config) { c.SSR = &SSR{URL: ssr.URL} }))
	r.GET("/", func(c *gin.Context) { Render(c, "Home", nil) })
	body := do(r, "GET", "/").Body.String()
	if !strings.Contains(body, "<title>SSR Home</title>") || !strings.Contains(body, "<h1>hi</h1>") {
		t.Fatal(body)
	}

	// SSR down → falls back to client rendering.
	ssr.Close()
	body = do(r, "GET", "/").Body.String()
	if !strings.Contains(body, `<script data-page="app"`) {
		t.Fatal("no CSR fallback")
	}
}

type createUser struct {
	Name    string `form:"name" binding:"required"`
	Email   string `json:"email" binding:"required,email"`
	Address struct {
		City string `json:"city" binding:"required"`
	} `json:"address"`
	Tags []struct {
		Label string `json:"label" binding:"required"`
	} `json:"tags" binding:"dive"`
}

func TestValidationErrors(t *testing.T) {
	r := gin.New()
	var got map[string]string
	r.POST("/", func(c *gin.Context) {
		var in createUser
		got = ValidationErrors(c.ShouldBindJSON(&in), in)
	})
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"email":"bad","tags":[{"label":""}]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(httptest.NewRecorder(), req)

	want := map[string]string{
		"name":         "The name field is required.",
		"email":        "The email must be a valid email address.",
		"address.city": "The address.city field is required.",
		"tags.0.label": "The tags.0.label field is required.",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q want %q (all: %v)", k, got[k], v, got)
		}
	}
}

func TestScrollProp(t *testing.T) {
	r := newRouter(newEngine(t))
	r.GET("/feed", func(c *gin.Context) {
		Render(c, "Feed", gin.H{"posts": ScrollPage([]int{1, 2}, 2, true).MatchOn("id")})
	})
	p := decodePage(t, do(r, "GET", "/feed", inertia))
	m := p.ScrollProps["posts"]
	if m.PageName != "page" || m.CurrentPage.(float64) != 2 || m.PreviousPage.(float64) != 1 || m.NextPage.(float64) != 3 || m.Reset {
		t.Fatalf("scroll meta %+v", m)
	}
	if p.Props["posts"].(map[string]any)["data"] == nil {
		t.Fatal("items missing")
	}
	if strings.Join(p.MergeProps, ",") != "posts.data" || strings.Join(p.MatchPropsOn, ",") != "posts.data.id" {
		t.Fatalf("append meta %v %v", p.MergeProps, p.MatchPropsOn)
	}
	p = decodePage(t, do(r, "GET", "/feed", inertia, hdr(HeaderMergeIntent, "prepend")))
	if strings.Join(p.PrependProps, ",") != "posts.data" || len(p.MergeProps) != 0 {
		t.Fatalf("prepend meta %v", p.PrependProps)
	}
	p = decodePage(t, do(r, "GET", "/feed", inertia, hdr(HeaderReset, "posts")))
	if !p.ScrollProps["posts"].Reset || len(p.MergeProps) != 0 {
		t.Fatal("reset not honored")
	}
	// Last page: no next.
	r.GET("/last", func(c *gin.Context) { Render(c, "Feed", gin.H{"posts": ScrollPage([]int{}, 1, false)}) })
	p = decodePage(t, do(r, "GET", "/last", inertia))
	if p.ScrollProps["posts"].NextPage != nil || p.ScrollProps["posts"].PreviousPage != nil {
		t.Fatalf("%+v", p.ScrollProps["posts"])
	}
}

func TestPrefetchKeepsFlash(t *testing.T) {
	r := newRouter(newEngine(t))
	r.POST("/save", func(c *gin.Context) { Flash(c, "success", "ok"); Redirect(c, "/") })
	r.GET("/", func(c *gin.Context) { Render(c, "P", nil) })
	cookie := do(r, "POST", "/save", inertia).Result().Cookies()[0]
	withCookie := func(req *http.Request) { req.AddCookie(cookie) }

	w := do(r, "GET", "/", inertia, withCookie, hdr("Purpose", "prefetch"))
	if p := decodePage(t, w); len(p.Flash) != 0 || len(w.Result().Cookies()) != 0 {
		t.Fatal("prefetch consumed the flash")
	}
	if p := decodePage(t, do(r, "GET", "/", inertia, withCookie)); p.Flash["success"] != "ok" {
		t.Fatal("flash lost")
	}
}

func mapFS(view string) fstest.MapFS { return fstest.MapFS{"app.html": {Data: []byte(view)}} }

func t0() *testing.T { return &testing.T{} }
