// Package ginertia is an Inertia.js (protocol v3) server adapter for Gin.
//
// Controllers render Svelte/Vue/React page components and pass Go values
// straight in as component props, the same way Laravel's Inertia::render works:
//
//	r.Use(inertia.Middleware())
//	r.GET("/users", func(c *gin.Context) {
//		ginertia.Render(c, "Users/Index", gin.H{"users": users})
//	})
package ginertia

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// Protocol headers.
const (
	HeaderInertia          = "X-Inertia"
	HeaderVersion          = "X-Inertia-Version"
	HeaderLocation         = "X-Inertia-Location"
	HeaderRedirect         = "X-Inertia-Redirect"
	HeaderPartialComponent = "X-Inertia-Partial-Component"
	HeaderPartialData      = "X-Inertia-Partial-Data"
	HeaderPartialExcept    = "X-Inertia-Partial-Except"
	HeaderErrorBag         = "X-Inertia-Error-Bag"
	HeaderReset            = "X-Inertia-Reset"
	HeaderExceptOnceProps  = "X-Inertia-Except-Once-Props"
)

// Props is the props map passed to a page component. gin.H works too.
type Props = map[string]any

// Config configures an Engine.
type Config struct {
	// RootView is the root HTML template path (default "views/app.html").
	// It is read from RootFS when set (e.g. an embed.FS), else from disk.
	RootView string
	RootFS   fs.FS
	// ReloadRootView re-parses the root template on every full page load.
	// Turn on in development.
	ReloadRootView bool
	// Funcs are extra template functions available in the root view.
	Funcs template.FuncMap

	// Version is the asset version. When empty and Vite is set, it is the
	// md5 of the Vite manifest. VersionFunc wins over Version when set.
	Version     string
	VersionFunc func() string

	// Vite enables the {{ vite "src/app.js" }} template helper.
	Vite *Vite

	// SSR enables server-side rendering through the Inertia SSR server.
	SSR *SSR

	// Flash stores validation errors and flash data between a redirect and
	// the next request. Defaults to a cookie store.
	Flash FlashStore

	// EncryptHistory turns on history encryption for every page.
	EncryptHistory bool

	// ContainerID is the root element id (default "app").
	ContainerID string

	// Marshal encodes the page object (default encoding/json).
	Marshal func(v any) ([]byte, error)

	// ErrorHandler is called when a prop resolver fails. Default: 500.
	ErrorHandler func(c *gin.Context, err error)
}

// Engine is the Inertia adapter. Create one with New and register
// Middleware on your Gin router.
type Engine struct {
	cfg Config

	sharedMu sync.RWMutex
	shared   Props

	tmplMu sync.RWMutex
	tmpl   *template.Template

	versionOnce sync.Once
	version     string
}

// New creates an Engine.
func New(cfg Config) (*Engine, error) {
	if cfg.RootView == "" {
		cfg.RootView = "views/app.html"
	}
	if cfg.ContainerID == "" {
		cfg.ContainerID = "app"
	}
	if cfg.Marshal == nil {
		cfg.Marshal = json.Marshal
	}
	if cfg.Flash == nil {
		cfg.Flash = &CookieFlashStore{}
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = func(c *gin.Context, err error) {
			_ = c.Error(err)
			c.AbortWithStatus(http.StatusInternalServerError)
		}
	}
	e := &Engine{cfg: cfg, shared: Props{}}
	if _, err := e.rootTemplate(); err != nil {
		return nil, err
	}
	return e, nil
}

// MustNew is New that panics on error.
func MustNew(cfg Config) *Engine {
	e, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return e
}

// Share adds a prop shared with every page. The value may be a resolver
// func (func() any, func(*gin.Context) any, with or without an error) so it
// is evaluated per request.
func (e *Engine) Share(key string, value any) {
	e.sharedMu.Lock()
	e.shared[key] = value
	e.sharedMu.Unlock()
}

func (e *Engine) sharedProps() Props {
	e.sharedMu.RLock()
	defer e.sharedMu.RUnlock()
	return maps.Clone(e.shared)
}

// Version returns the current asset version.
func (e *Engine) Version() string {
	if e.cfg.VersionFunc != nil {
		return e.cfg.VersionFunc()
	}
	if e.cfg.Version != "" {
		return e.cfg.Version
	}
	if e.cfg.Vite == nil || e.cfg.Vite.IsDev() {
		return ""
	}
	e.versionOnce.Do(func() {
		if b, err := e.cfg.Vite.readManifest(); err == nil {
			sum := md5.Sum(b)
			e.version = hex.EncodeToString(sum[:])
		}
	})
	return e.version
}

func (e *Engine) rootTemplate() (*template.Template, error) {
	if !e.cfg.ReloadRootView {
		e.tmplMu.RLock()
		t := e.tmpl
		e.tmplMu.RUnlock()
		if t != nil {
			return t, nil
		}
	}
	var (
		b   []byte
		err error
	)
	if e.cfg.RootFS != nil {
		b, err = fs.ReadFile(e.cfg.RootFS, e.cfg.RootView)
	} else {
		b, err = os.ReadFile(e.cfg.RootView)
	}
	if err != nil {
		return nil, fmt.Errorf("ginertia: read root view: %w", err)
	}
	funcs := template.FuncMap{
		"vite": func(entries ...string) (template.HTML, error) {
			if e.cfg.Vite == nil {
				return "", fmt.Errorf("ginertia: Config.Vite is not set")
			}
			return e.cfg.Vite.Tags(entries...)
		},
		"viteAsset": func(path string) (string, error) {
			if e.cfg.Vite == nil {
				return "", fmt.Errorf("ginertia: Config.Vite is not set")
			}
			return e.cfg.Vite.Asset(path)
		},
	}
	maps.Copy(funcs, e.cfg.Funcs)
	t, err := template.New("root").Funcs(funcs).Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("ginertia: parse root view: %w", err)
	}
	e.tmplMu.Lock()
	e.tmpl = t
	e.tmplMu.Unlock()
	return t, nil
}

// ---------------------------------------------------------------------------
// Middleware & per-request state
// ---------------------------------------------------------------------------

const stateKey = "ginertia.state"

type state struct {
	e              *Engine
	shared         Props
	view           map[string]any
	incoming       *FlashData // flash from the previous request
	outgoing       *FlashData // flash for the next request
	encryptHistory *bool
	clearHistory   bool
}

// Middleware must be registered before any route that calls Render.
func (e *Engine) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		st := &state{e: e, shared: Props{}, view: map[string]any{}}
		c.Set(stateKey, st)
		addVary(c.Writer.Header(), HeaderInertia)

		// A prefetch (hover on <Link prefetch>) must not consume flash
		// data meant for the page the user is actually on.
		if c.GetHeader("Purpose") != "prefetch" {
			if in, err := e.cfg.Flash.Read(c); err == nil && in != nil && !in.empty() {
				st.incoming = in
			}
		}

		if IsInertia(c) && c.Request.Method == http.MethodGet &&
			c.GetHeader(HeaderVersion) != e.Version() {
			// Asset version changed: force a full page reload and keep
			// the flash data alive for that reload.
			if st.incoming != nil {
				_ = e.cfg.Flash.Write(c, st.incoming)
			}
			// Relative URL: never trust the Host header (host injection).
			c.Header(HeaderLocation, requestURI(c.Request))
			c.AbortWithStatus(http.StatusConflict)
			return
		}

		c.Next()

		// 302 after PUT/PATCH/DELETE must become 303 or the browser
		// repeats the request with the same method.
		if IsInertia(c) && c.Writer.Status() == http.StatusFound && !c.Writer.Written() {
			switch c.Request.Method {
			case http.MethodPut, http.MethodPatch, http.MethodDelete:
				c.Writer.WriteHeader(http.StatusSeeOther)
			}
		}
	}
}

func getState(c *gin.Context) *state {
	v, ok := c.Get(stateKey)
	if !ok {
		panic("ginertia: Middleware() is not registered on this route")
	}
	return v.(*state)
}

// IsInertia reports whether the request was made by the Inertia client.
func IsInertia(c *gin.Context) bool {
	return c.GetHeader(HeaderInertia) == "true"
}

// Share adds a prop for the current request only (e.g. the logged-in user,
// from your auth middleware).
func Share(c *gin.Context, key string, value any) {
	getState(c).shared[key] = value
}

// ViewData passes data to the root HTML template only (not to the page
// component), e.g. a <title> or meta tags for full page loads.
func ViewData(c *gin.Context, key string, value any) {
	getState(c).view[key] = value
}

// EncryptHistory overrides history encryption for the current response.
func EncryptHistory(c *gin.Context, encrypt bool) {
	getState(c).encryptHistory = &encrypt
}

// ClearHistory clears the client's encrypted history on the current
// response, or on the next one if you redirect.
func ClearHistory(c *gin.Context) {
	st := getState(c)
	st.clearHistory = true
	st.out().ClearHistory = true
	st.persist(c)
}

// requestURI returns the request path + query as a relative URL. Leading
// slashes are collapsed so a request for "//evil.example/x" is never echoed
// back as a protocol-relative URL (the client assigns it to window.location
// on a 409 and to history on every page).
func requestURI(r *http.Request) string {
	u := r.URL.RequestURI()
	for strings.HasPrefix(u, "//") {
		u = u[1:]
	}
	return u
}

// addVary appends to Vary instead of overwriting it, so values set by
// other middleware (e.g. Accept-Encoding from gzip) survive.
func addVary(h http.Header, value string) {
	for _, v := range h.Values("Vary") {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return
			}
		}
	}
	h.Add("Vary", value)
}
