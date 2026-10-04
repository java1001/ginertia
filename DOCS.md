# ginertia — Technical Reference

> Complete integration reference for **ginertia**, an Inertia.js v3 server adapter for the Gin web framework (Go).
> Written for developers and AI coding agents. Everything here is derived from the library source; where behavior is subtle, the exact rule is stated.

- Module: `github.com/java1001/ginertia`
- Source: https://github.com/java1001/ginertia
- Package: `ginertia`
- Requires: Go 1.24+, Gin 1.11+, `github.com/go-playground/validator/v10`
- Client: `@inertiajs/svelte` 3.x (tested 3.8 with Svelte 5, Vite 8); `@inertiajs/vue3` / `@inertiajs/react` 3.x use the same server side
- Docs site: https://ginertia.org

---

## Table of contents

1. [Concepts](#1-concepts)
2. [Project layout](#2-project-layout)
3. [Installation](#3-installation)
4. [Minimal working app](#4-minimal-working-app)
5. [Engine and Config](#5-engine-and-config)
6. [Middleware and request lifecycle](#6-middleware-and-request-lifecycle)
7. [Rendering](#7-rendering)
8. [Props: values, resolvers and prop types](#8-props-values-resolvers-and-prop-types)
9. [Shared props](#9-shared-props)
10. [Redirects](#10-redirects)
11. [Validation errors and flash data](#11-validation-errors-and-flash-data)
12. [History encryption](#12-history-encryption)
13. [Root template and ViewData](#13-root-template-and-viewdata)
14. [Vite integration](#14-vite-integration)
15. [Server-side rendering (SSR)](#15-server-side-rendering-ssr)
16. [CSRF protection](#16-csrf-protection)
17. [Flash stores](#17-flash-stores)
18. [Wire protocol reference](#18-wire-protocol-reference)
19. [Recipes](#19-recipes)
20. [Testing your handlers](#20-testing-your-handlers)
21. [Production deployment](#21-production-deployment)
22. [Security model](#22-security-model)
23. [Limitations](#23-limitations)
24. [Troubleshooting](#24-troubleshooting)
25. [Complete exported API](#25-complete-exported-api)
26. [Integration checklist](#26-integration-checklist)

---

## 1. Concepts

Inertia lets a server-side framework drive a client-side SPA without a JSON API:

1. A Gin handler calls `ginertia.Render(c, "Users/Index", props)`.
2. **First visit** (normal browser request): the server returns a full HTML document. The page object (component name + props) is embedded as JSON in `<script data-page="app" type="application/json">`. The client boots and mounts the component.
3. **Subsequent visits** (clicking `<Link>`, `router.visit`, form submits): the client sends `X-Inertia: true`. The server returns the page object as JSON. The client swaps components without a full reload.
4. Forms POST/PUT/PATCH/DELETE to normal Gin routes. On validation failure the handler stores errors and redirects back; the client receives the errors as the `errors` prop.

Mapping to Laravel:

| Laravel | ginertia |
|---|---|
| `Inertia::render('Users/Index', [...])` | `ginertia.Render(c, "Users/Index", gin.H{...})` |
| `Inertia::share('key', $v)` | `engine.Share("key", v)` (global) · `ginertia.Share(c, "key", v)` (per request) |
| `fn () => ...` | `func() any`, `func(*gin.Context) (any, error)`, or a typed func such as `func() []User` |
| `Inertia::optional(fn)` | `ginertia.Optional(fn)` (alias `Lazy`) |
| `Inertia::always($v)` | `ginertia.Always(v)` |
| `Inertia::defer(fn, 'group')` | `ginertia.Defer(fn, "group")` · `.Merge()` · `.DeepMerge()` |
| `Inertia::merge($v)` | `ginertia.Merge(v)` · `.Prepend()` · `.MatchOn("id")` · `ginertia.DeepMerge(v)` |
| `Inertia::once(fn)` | `ginertia.Once(fn).Until(time.Hour)` |
| `Inertia::scroll($paginator)` | `ginertia.ScrollPage(items, page, hasMore)` · `ginertia.Scroll(v, ScrollMeta{...})` |
| `back()->withErrors($e)` | `ginertia.WithErrors(c, errs); ginertia.Back(c)` |
| `$request->validate()` | `ginertia.ValidationErrors(c.ShouldBind(&in), in)` |
| `session()->flash('k', v)` | `ginertia.Flash(c, "k", v)` → `page.flash.k` |
| `redirect('/users')` | `ginertia.Redirect(c, "/users")` |
| `Inertia::location($url)` | `ginertia.Location(c, url)` |
| `Inertia::encryptHistory()` / `clearHistory()` | `ginertia.EncryptHistory(c, true)` / `ginertia.ClearHistory(c)` |
| `view()->with(...)` | `ginertia.ViewData(c, "title", "...")` |
| `VerifyCsrfToken` | `ginertia.CSRF(ginertia.CSRFConfig{Secret: key})` |
| `laravel-vite-plugin` | `vite-plugin-ginertia.js` (copied from `example/`) |

---

## 2. Project layout

Recommended layout (matches the example app and all defaults):

```
myapp/
├── main.go
├── handlers.go
├── views/
│   └── app.html                 # root template (Config.RootView)
├── frontend/
│   ├── app.js                   # client entry (Vite input)
│   ├── ssr.js                   # optional SSR entry
│   ├── Layout.svelte
│   └── Pages/
│       ├── Home.svelte          # Render(c, "Home", ...)
│       └── Users/
│           └── Index.svelte     # Render(c, "Users/Index", ...)
├── public/
│   ├── .gitkeep                 # keeps public/ present for go:embed
│   ├── hot                      # written by Vite in dev (gitignored)
│   └── build/                   # vite build output (gitignored)
│       └── .vite/manifest.json
├── bootstrap/ssr/ssr.js         # vite build --ssr output (gitignored)
├── vite.config.js
├── vite-plugin-ginertia.js
└── package.json
```

**Component naming rule:** the string passed to `Render` is resolved by the client's `resolve` function. With the standard resolver, `"Users/Index"` → `./Pages/Users/Index.svelte`. The server never checks that the component exists.

Suggested `.gitignore`:

```
node_modules/
public/build/
public/hot
bootstrap/
```

---

## 3. Installation

```bash
go get github.com/java1001/ginertia
npm i -D vite @sveltejs/vite-plugin-svelte svelte @inertiajs/svelte
```

Copy `example/vite-plugin-ginertia.js` into the project root. It is not published to npm.

`package.json` scripts:

```json
{
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vite build && vite build --ssr",
    "ssr": "node bootstrap/ssr/ssr.js"
  }
}
```

Drop `&& vite build --ssr` and the `ssr` script if you don't use SSR.

---

## 4. Minimal working app

### `main.go`

```go
package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/java1001/ginertia"
)

//go:embed views all:public
var embedded embed.FS

func main() {
	prod := os.Getenv("APP_ENV") == "production"

	// Dev: read views and manifest from disk. Prod: from the binary.
	var files fs.FS = os.DirFS(".")
	if prod {
		files = embedded
		gin.SetMode(gin.ReleaseMode)
	}

	key := []byte(os.Getenv("APP_KEY")) // same value on every instance

	inertia := ginertia.MustNew(ginertia.Config{
		RootView:       "views/app.html",
		RootFS:         files,
		ReloadRootView: !prod,
		Vite:           &ginertia.Vite{FS: files, DisableDev: prod},
		Flash:          &ginertia.CookieFlashStore{Secret: key, Secure: prod},
	})
	inertia.Share("appName", "My App")

	r := gin.Default()
	build, _ := fs.Sub(files, "public/build")
	r.StaticFS("/build", http.FS(build))

	r.Use(ginertia.CSRF(ginertia.CSRFConfig{Secret: key, Secure: prod}))
	r.Use(inertia.Middleware())

	r.GET("/", func(c *gin.Context) {
		ginertia.Render(c, "Home", gin.H{"message": "Hello from Go"})
	})

	log.Fatal(r.Run(":8080"))
}
```

### `views/app.html`

```html
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  {{ if not .inertiaHead }}<title>{{ if .title }}{{ .title }} · {{ end }}My App</title>{{ end }}
  {{ vite "frontend/app.js" }}
  {{ .inertiaHead }}
</head>
<body>
  {{ .inertia }}
</body>
</html>
```

### `vite.config.js`

```js
import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import ginertia from './vite-plugin-ginertia.js'

export default defineConfig({
  plugins: [
    svelte(),
    ginertia({ input: 'frontend/app.js', ssr: 'frontend/ssr.js' }),
  ],
})
```

### `frontend/app.js`

```js
import { createInertiaApp, router } from '@inertiajs/svelte'
import Layout from './Layout.svelte'

// Drop prefetched pages when the server clears history (logout).
router.on('navigate', (event) => {
  if (event.detail.page.clearHistory) router.flushAll()
})

createInertiaApp({
  resolve: (name) => {
    const pages = import.meta.glob('./Pages/**/*.svelte', { eager: true })
    const page = pages[`./Pages/${name}.svelte`]
    if (!page) throw new Error(`Page not found: ${name}`)
    return page
  },
  layout: () => Layout,
})
```

### `frontend/Pages/Home.svelte`

```svelte
<script>
  let { message } = $props()
</script>
<h1>{message}</h1>
```

### Run

```bash
# Development (two terminals)
npm run dev        # Vite + HMR, writes public/hot
go run .           # http://localhost:8080

# Production
npm run build
APP_ENV=production go build -o app . && APP_ENV=production APP_KEY=... ./app
```

### Vue / React entry points

The Go side is identical. Only the client package, the Vite framework plugin and the entry file change. These are standard Inertia client setups and are not covered by ginertia's own test suite.

```js
// frontend/app.js — Vue 3 (npm i -D @vitejs/plugin-vue vue @inertiajs/vue3)
import { createApp, h } from 'vue'
import { createInertiaApp } from '@inertiajs/vue3'

createInertiaApp({
  resolve: (name) => import.meta.glob('./Pages/**/*.vue', { eager: true })[`./Pages/${name}.vue`],
  setup({ el, App, props, plugin }) {
    createApp({ render: () => h(App, props) }).use(plugin).mount(el)
  },
})
```

```jsx
// frontend/app.jsx — React (npm i -D @vitejs/plugin-react react react-dom @inertiajs/react)
import { createRoot } from 'react-dom/client'
import { createInertiaApp } from '@inertiajs/react'

createInertiaApp({
  resolve: (name) => import.meta.glob('./Pages/**/*.jsx', { eager: true })[`./Pages/${name}.jsx`],
  setup({ el, App, props }) {
    createRoot(el).render(<App {...props} />)
  },
})
```

For React, use `{{ vite "frontend/app.jsx" }}` in the root view and `input: 'frontend/app.jsx'` in the Vite plugin.

---

## 5. Engine and Config

```go
func New(cfg Config) (*Engine, error)
func MustNew(cfg Config) *Engine // panics on error
```

`New` applies defaults and parses the root template immediately. It returns an error if the root view cannot be read or parsed.

```go
type Config struct {
	RootView       string                          // default "views/app.html"
	RootFS         fs.FS                           // read RootView from here; nil = os.ReadFile
	ReloadRootView bool                            // re-parse the template on every full page load (dev)
	Funcs          template.FuncMap                // extra funcs for the root template

	Version     string                             // asset version
	VersionFunc func() string                      // wins over Version

	Vite  *Vite                                    // enables {{ vite }} / {{ viteAsset }}
	SSR   *SSR                                     // enables server-side rendering
	Flash FlashStore                               // default &CookieFlashStore{} (random key)

	EncryptHistory bool                            // global default for page.encryptHistory
	ContainerID    string                          // default "app"
	Marshal        func(v any) ([]byte, error)     // default encoding/json.Marshal
	ErrorHandler   func(c *gin.Context, err error) // default: c.Error(err) + 500, empty body
}
```

### Asset version resolution (`Engine.Version()`)

Evaluated in this order:

1. `VersionFunc()` if set.
2. `Version` if non-empty.
3. `""` if `Vite` is nil or Vite is in dev mode.
4. Otherwise the hex md5 of the Vite manifest file, **computed once per process** and cached.

Because the manifest hash is cached, a process must be restarted after a new `vite build` for the version to change. This is the normal deploy flow.

### Engine methods

```go
func (e *Engine) Middleware() gin.HandlerFunc
func (e *Engine) Share(key string, value any)        // global shared prop, goroutine-safe
func (e *Engine) Version() string
func (e *Engine) Render(c *gin.Context, component string, props map[string]any)
func (e *Engine) BuildPage(c *gin.Context, component string, props map[string]any) (*Page, error)
```

`Engine` is safe for concurrent use. Create one per application and reuse it.

---

## 6. Middleware and request lifecycle

```go
r.Use(inertia.Middleware())
```

**Must be registered before any route that calls `Render`, `Share(c, …)`, `Flash`, `WithErrors`, `ViewData`, `EncryptHistory` or `ClearHistory`.** Otherwise these functions panic with `ginertia: Middleware() is not registered on this route`.

Recommended order:

```go
r.Use(ginertia.CSRF(...))      // 1. CSRF (optional but recommended)
r.Use(sessions.Sessions(...))  // 2. your session middleware, if a server-side FlashStore needs it
r.Use(inertia.Middleware())    // 3. ginertia
r.Use(auth.ShareUser)          // 4. middleware that calls ginertia.Share(c, ...)
```

### What the middleware does, per request

Before the handler:

1. Creates per-request state and stores it in the Gin context (key `"ginertia.state"`).
2. Appends `X-Inertia` to the `Vary` response header (never overwrites existing values).
3. Reads (and consumes) flash data from the `FlashStore`, **unless** the request has `Purpose: prefetch`.
4. If the request is an Inertia `GET` (`X-Inertia: true`) and `X-Inertia-Version` ≠ `Engine.Version()`:
   - re-writes any consumed flash data so it survives the reload,
   - responds `409 Conflict` with `X-Inertia-Location: <request URI>` (relative URL; leading slashes collapsed so `//evil.example` becomes `/evil.example`),
   - aborts. The client then performs a full page load.

After the handler:

5. If the request is an Inertia request, the response status is `302`, nothing has been written yet, and the method is `PUT`, `PATCH` or `DELETE`, the status is rewritten to `303 See Other`. This makes a plain `c.Redirect(http.StatusFound, …)` safe.

### `IsInertia`

```go
func IsInertia(c *gin.Context) bool // X-Inertia header == "true"
```

---

## 7. Rendering

```go
func Render(c *gin.Context, component string, props map[string]any)
```

`gin.H` and `ginertia.Props` (`= map[string]any`) both work. `nil` props are allowed.

Behavior:

- Builds the page object (see `BuildPage` below). If a prop resolver returns an error or marshaling fails, `Config.ErrorHandler` is called (default: 500 with an empty body; the error is recorded via `c.Error`).
- **Inertia request:** responds `200`, `Content-Type: application/json; charset=utf-8`, header `X-Inertia: true`, body = page JSON.
- **Normal request:** renders the root template and responds `200`, `Content-Type: text/html; charset=utf-8`.

After `Render`, do not write to the response again.

### `BuildPage`

```go
func (e *Engine) BuildPage(c *gin.Context, component string, props map[string]any) (*Page, error)
```

Builds the `*Page` without writing a response. Useful in tests or for custom responses. Side effect: consumes outgoing flash data set during this request (see §11).

Prop merge order (later wins):

1. Global shared props (`Engine.Share`)
2. Request shared props (`ginertia.Share(c, …)`)
3. Props passed to `Render`
4. `errors` is added automatically as `Always(map)` **only if** no prop named `errors` exists.

`sharedProps` in the page object lists the keys from steps 1–2 (sorted). Props are resolved in sorted key order.

### `Page` (the page object)

```go
type Page struct {
	Component      string                   `json:"component"`
	Props          Props                    `json:"props"`
	URL            string                   `json:"url"`            // request URI incl. query, leading slashes collapsed
	Version        string                   `json:"version"`
	EncryptHistory bool                     `json:"encryptHistory"`
	ClearHistory   bool                     `json:"clearHistory"`
	Flash          map[string]any           `json:"flash"`
	SharedProps    []string                 `json:"sharedProps,omitempty"`
	MergeProps     []string                 `json:"mergeProps,omitempty"`
	PrependProps   []string                 `json:"prependProps,omitempty"`
	DeepMergeProps []string                 `json:"deepMergeProps,omitempty"`
	MatchPropsOn   []string                 `json:"matchPropsOn,omitempty"`
	DeferredProps  map[string][]string      `json:"deferredProps,omitempty"`
	OnceProps      map[string]OnceMeta      `json:"onceProps,omitempty"`
	ScrollProps    map[string]ScrollMetaOut `json:"scrollProps,omitempty"`
}
```

Example JSON response:

```json
{
  "component": "Users/Index",
  "props": {
    "appName": "My App",
    "auth": { "user": { "email": "a@b.c", "name": "a" } },
    "errors": {},
    "users": [{ "id": 1, "name": "An" }],
    "page": 1,
    "hasMore": true
  },
  "url": "/users",
  "version": "4f1c0c2b9d0e…",
  "encryptHistory": true,
  "clearHistory": false,
  "flash": {},
  "sharedProps": ["appName", "auth"],
  "mergeProps": ["users"],
  "matchPropsOn": ["users.id"],
  "deferredProps": { "default": ["stats"] }
}
```

**Everything in `props` is sent to the browser** and serialized with `Config.Marshal` (struct `json` tags apply). Never pass structs with secret fields; use `json:"-"` or map to a DTO.

---

## 8. Props: values, resolvers and prop types

### 8.1 Plain values and resolvers

A prop value is either a plain value or a **resolver function**. Resolvers run only if the prop is actually included in the response, so expensive work is skipped on partial reloads that don't request it.

Accepted resolver signatures:

```go
func() any
func() (any, error)
func(*gin.Context) any
func(*gin.Context) (any, error)
// plus any typed equivalent, via reflection:
func() []User
func(*gin.Context) (*User, error)
```

Rules for typed funcs: zero args or exactly one `*gin.Context` arg; one return value, or two where the second is `error`. Any other function signature makes `Render` fail with `ginertia: unsupported resolver <type>` (→ `ErrorHandler`, 500). Non-function values pass through unchanged.

```go
ginertia.Render(c, "Dashboard", gin.H{
	"user":   user,                                         // plain value
	"time":   func() string { return time.Now().String() }, // typed resolver
	"orders": func(c *gin.Context) ([]Order, error) {       // may fail → 500
		return repo.Orders(c.Request.Context(), userID)
	},
})
```

### 8.2 Prop type wrappers

```go
func Optional(resolver any) OptionalProp
func Lazy(resolver any) OptionalProp           // alias of Optional
func Always(value any) AlwaysProp
func Defer(resolver any, group ...string) DeferProp
func (p DeferProp) Merge() DeferProp
func (p DeferProp) DeepMerge() DeferProp
func Merge(value any) MergeProp
func DeepMerge(value any) MergeProp
func (p MergeProp) Prepend() MergeProp
func (p MergeProp) MatchOn(fields ...string) MergeProp
func Once(resolver any) OnceProp
func (p OnceProp) Until(ttl time.Duration) OnceProp
func Scroll(value any, meta ScrollMeta) ScrollProp
func ScrollPage(items any, page int, hasMore bool) ScrollProp
func (p ScrollProp) Wrapper(key string) ScrollProp
func (p ScrollProp) PageName(name string) ScrollProp
func (p ScrollProp) MatchOn(fields ...string) ScrollProp
```

The wrapped value of any wrapper may itself be a plain value or a resolver.

### 8.3 Inclusion rules

A request is **partial** when it is an Inertia request **and** `X-Inertia-Partial-Component` equals the component being rendered. If the component name differs (e.g. the user navigated elsewhere), the request is treated as a normal visit.

On a partial request, for every prop **except `Always`**:

- if `X-Inertia-Partial-Data` (`only`) is non-empty and the key is not in it → omitted;
- if the key is in `X-Inertia-Partial-Except` (`except`) → omitted.

Then, per type:

| Type | Normal visit | Partial visit (passes only/except) | Page metadata emitted |
|---|---|---|---|
| plain value / resolver | sent | sent | — |
| `Always(v)` | sent | **always sent**, ignores only/except | — |
| `Optional(fn)` | **omitted** | sent | — |
| `Defer(fn, group)` | **omitted**, resolver not run | sent | normal visit: `deferredProps[group]` += key. Partial visit with `.Merge()`/`.DeepMerge()`: `mergeProps`/`deepMergeProps` += key (unless key in `X-Inertia-Reset`) |
| `Merge(v)` | sent | sent | `mergeProps` (or `prependProps` with `.Prepend()`, `deepMergeProps` for `DeepMerge`) += key; `matchPropsOn` += `key.field` per `MatchOn` field. Skipped if key in `X-Inertia-Reset` |
| `Once(fn)` | sent, unless key in `X-Inertia-Except-Once-Props` | same; but if the key is explicitly in `only`, it is sent even when the client already holds it | `onceProps[key] = {prop, expiresAt}`; `expiresAt` = now + TTL in Unix ms, or `null` without `Until` |
| `Scroll(...)` / `ScrollPage(...)` | sent | sent | `scrollProps[key] = {pageName, previousPage, nextPage, currentPage, reset}`; `mergeProps` (or `prependProps` if `X-Inertia-Infinite-Scroll-Merge-Intent: prepend`) += `key.<wrapper>`; `matchPropsOn` += `key.<wrapper>.field`. Merge metadata skipped if key in `X-Inertia-Reset` |

Defer groups: default group is `"default"`. Props in the same group are fetched by the client in one request.

`Optional` and `Defer` are **not** access control: any client can request them with a partial reload. Check permissions in the handler.

### 8.4 Scroll props in detail

```go
type ScrollMeta struct {
	PageName     string `json:"pageName"`     // query param, default "page"
	PreviousPage any    `json:"previousPage"` // nil = no previous page
	NextPage     any    `json:"nextPage"`     // nil = no next page
	CurrentPage  any    `json:"currentPage"`
}
```

`ScrollPage(items, page, hasMore)` produces value `{"data": items}` and meta: `CurrentPage = page`, `PreviousPage = page-1` if `page > 1`, `NextPage = page+1` if `hasMore`. To know `hasMore`, fetch `perPage+1` rows.

`Scroll(value, meta)` is for cursor pagination or custom shapes. `value` must contain the items array under the wrapper key (default `"data"`; change with `.Wrapper("items")`). Page identifiers may be ints or cursor strings.

---

## 9. Shared props

```go
inertia.Share("appName", "My App")                       // global, every page, every request
inertia.Share("locale", func(c *gin.Context) any {       // global resolver, evaluated per request
	return c.GetHeader("Accept-Language")
})

ginertia.Share(c, "auth", gin.H{"user": currentUser})    // current request only
```

- `Engine.Share` is goroutine-safe and is normally called at startup.
- `ginertia.Share(c, …)` is normally called from middleware registered **after** `inertia.Middleware()`.
- Props passed to `Render` override shared props with the same key.
- Shared keys are listed in `page.sharedProps`.
- On the client: `page.props.appName` (Svelte: `import { page } from '@inertiajs/svelte'`).

---

## 10. Redirects

```go
func Redirect(c *gin.Context, location string)
func Back(c *gin.Context, fallback ...string)
func Location(c *gin.Context, target string)
func IsLocalURL(target string) bool
```

### `Redirect`

Internal redirect. Status `302` for `GET`, `303` for any other method (Inertia requires 303 after non-GET so the browser follows with GET). Calls `c.Abort()`.

A plain `c.Redirect(http.StatusFound, "/users")` also works after PUT/PATCH/DELETE: the middleware upgrades it to 303 for Inertia requests.

### `Back`

Redirects to the `Referer`, or to `fallback` (default `"/"`). The referer is followed only if:

- it has the same host as the request and an `http`/`https` scheme, **or** it is a path-only reference starting with `/`;
- the resulting path does not start with `//` or `/\` (protocol-relative).

The redirect target is reduced to path + query. Cross-site referers fall back.

### `Location`

Forces a full browser navigation (`window.location`). Use it for external URLs, OAuth providers, file downloads or non-Inertia pages.

- Inertia request → `409` + `X-Inertia-Location: <target>`, aborted.
- Normal request → `302` to target, aborted.
- Rejected with `500` (error recorded via `c.Error`) if the URL fails to parse or its scheme is not empty/`http`/`https` (e.g. `javascript:`).
- **The host is not restricted** (`//evil.example` and `https://evil.example` are accepted — that is the point of `Location`). Never pass user input such as `?next=` / `?redirect=` straight to `Location` or `Redirect`: that is an open redirect. Validate it first:

```go
func IsLocalURL(target string) bool
```

`IsLocalURL` is true only for paths on this site (`/dashboard`, `/a?b=c`); it rejects absolute URLs, `//host`, `/\host`, `\\host`, schemes and control characters (browsers strip tabs/newlines, turning `/\t/evil` into `//evil`).

```go
next := c.Query("next")
if !ginertia.IsLocalURL(next) {
	next = "/dashboard"
}
ginertia.Redirect(c, next)
```

---

## 11. Validation errors and flash data

```go
func WithErrors[V any](c *gin.Context, errs map[string]V)
func Flash(c *gin.Context, key string, value any)
func ValidationErrors(err error, obj any, msg ...MessageFunc) map[string]string

type MessageFunc func(fe validator.FieldError, field string) string
var DefaultMessage MessageFunc
```

### How it flows

1. A handler calls `WithErrors` and/or `Flash`, then redirects (`Back` / `Redirect`).
2. The data is written immediately to the `FlashStore` (default: signed cookie).
3. On the next request, the middleware reads and deletes it.
4. `Render` exposes errors as the `errors` prop (always present, `{}` when empty) and flash data as `page.flash`.

If you call `WithErrors`/`Flash` and then `Render` **in the same request** (no redirect), the data is included in that response and the store entry is cleared.

Flash data also survives: a 409 version-mismatch reload, and prefetch requests (`Purpose: prefetch` does not consume it).

### `WithErrors`

Accepts any `map[string]V` (`map[string]string`, `map[string][]string`, …). Calling it again in the same request replaces the previous errors.

Error bags: if the request has `X-Inertia-Error-Bag: <bag>` (set by `useForm` / router option `errorBag`), errors are nested as `errors[<bag>][field]`.

### `Flash`

Sets `page.flash[key] = value` for the next page. Multiple keys accumulate.

Client side:

```svelte
<script>
  import { page, router } from '@inertiajs/svelte'
  // read: page.flash.success
  // or react to it:
  $effect(() => router.on('flash', (e) => showToast(e.detail.flash.success)))
</script>
```

### `ValidationErrors`

Converts the error from `c.ShouldBind(&in)` / `c.ShouldBindJSON(&in)` into `field → message`:

- `err == nil` → returns `nil`.
- Not a `validator.ValidationErrors` (malformed JSON, type mismatch, …) → `{"_": "The request could not be read."}`. The raw parser error is never exposed.
- Field names come from the `form` tag, then the `json` tag, then the Go field name. Nested structs → `address.city`; slices → `tags.0.label`. The root struct name is dropped.
- Only the first failing rule per field is kept.
- `obj` may be the struct value or a pointer to it.

Built-in messages (`DefaultMessage`): `required`, `email`, `url`/`http_url`, `min`/`gte`, `max`/`lte` (string/slice → "characters"), `len`, `oneof`, `eqfield`; anything else → `"The <field> is invalid."`. Underscores in field names become spaces.

Custom messages, per call or app-wide:

```go
msg := func(fe validator.FieldError, field string) string {
	switch fe.Tag() {
	case "required":
		return field + " là bắt buộc."
	}
	return ginertia.DefaultMessage(fe, field)
}

ginertia.ValidationErrors(err, in, msg) // this call only
ginertia.DefaultMessage = msg           // app-wide (set once at startup)
```

### Full form example

```go
type UserForm struct {
	Name  string `json:"name"  form:"name"  binding:"required,min=2,max=50"`
	Email string `json:"email" form:"email" binding:"required,email"`
	Role  string `json:"role"  form:"role"  binding:"required,oneof=admin editor viewer"`
}

func (h *handlers) usersStore(c *gin.Context) {
	var in UserForm
	if err := c.ShouldBind(&in); err != nil {
		ginertia.WithErrors(c, ginertia.ValidationErrors(err, in))
		ginertia.Back(c)
		return
	}
	if h.store.emailTaken(in.Email) {
		ginertia.WithErrors(c, map[string]string{"email": "This email is already taken."})
		ginertia.Back(c)
		return
	}
	u := h.store.create(in)
	ginertia.Flash(c, "success", "Created "+u.Name+".")
	ginertia.Redirect(c, "/users")
}
```

```svelte
<script>
  import { useForm } from '@inertiajs/svelte'
  let { user } = $props()
  const form = useForm(() => ({ name: user?.name ?? '', email: user?.email ?? '', role: user?.role ?? 'viewer' }))
  function submit(e) {
    e.preventDefault()
    user ? form.put(`/users/${user.id}`) : form.post('/users')
  }
</script>

<form onsubmit={submit} novalidate>
  <input bind:value={form.name} />
  {#if form.errors.name}<small>{form.errors.name}</small>{/if}
  <input type="email" bind:value={form.email} />
  {#if form.errors.email}<small>{form.errors.email}</small>{/if}
  <button disabled={form.processing}>Save</button>
</form>
```

`useForm` sends JSON; the `<Form>` component sends form data. `c.ShouldBind` handles both (by `Content-Type`), which is why tags include both `json` and `form`.

---

## 12. History encryption

```go
func EncryptHistory(c *gin.Context, encrypt bool) // per response; overrides Config.EncryptHistory
func ClearHistory(c *gin.Context)
```

- `Config.EncryptHistory = true` turns it on globally. `EncryptHistory(c, …)` overrides it for the current response (typically from an auth middleware for logged-in routes).
- `ClearHistory(c)` sets `page.clearHistory = true` on the current response **and** stores it in the flash store, so it also applies to the page after a redirect (e.g. logout → redirect to `/login`).

Logout pattern:

```go
func logout(c *gin.Context) {
	sessions.Logout(c)
	ginertia.ClearHistory(c)
	ginertia.Redirect(c, "/login")
}
```

And on the client (prefetch cache is client-side and must be flushed there):

```js
router.on('navigate', (e) => { if (e.detail.page.clearHistory) router.flushAll() })
```

---

## 13. Root template and ViewData

The root view is an `html/template`. Data available in it:

| Key | Type | Content |
|---|---|---|
| `.inertia` | `template.HTML` | CSR: `<script data-page="<ContainerID>" type="application/json">{page JSON}</script><div id="<ContainerID>"></div>`. SSR: the SSR body |
| `.inertiaHead` | `template.HTML` | SSR head tags joined by newlines; empty without SSR |
| `.page` | `*ginertia.Page` | the page object (e.g. `{{ .page.Component }}`, `{{ .page.Props.appName }}`) |
| any `ViewData` key | any | values set with `ginertia.ViewData` |

`page`, `inertia` and `inertiaHead` are set after `ViewData` values, so they cannot be overridden.

Template functions:

| Function | Description |
|---|---|
| `{{ vite "frontend/app.js" }}` | Script/style tags for one or more entries (dev server or manifest). Errors if `Config.Vite` is nil |
| `{{ viteAsset "frontend/img/logo.png" }}` | Public URL of an asset processed by Vite |
| anything in `Config.Funcs` | your own functions (can override the two above) |

```go
func ViewData(c *gin.Context, key string, value any)
```

`ViewData` passes data to the root template only, never to the component. Use it for `<title>`, meta tags or Open Graph tags on the first page load:

```go
ginertia.ViewData(c, "title", "Users")
ginertia.Render(c, "Users/Index", props)
```

`ReloadRootView: true` re-reads and re-parses the template on every HTML response (development). Otherwise it is parsed once in `New`.

---

## 14. Vite integration

```go
type Vite struct {
	DisableDev   bool   // never use the dev server (ignore HotFile/DevURL) — set it in production
	HotFile      string // default "public/hot"
	DevURL       string // force dev mode, e.g. "http://localhost:5173"
	BaseURL      string // public URL of the build dir, default "/build/"
	ManifestPath string // default "public/build/.vite/manifest.json"
	FS           fs.FS  // read the manifest from here (e.g. embed.FS) instead of disk
}

func (v *Vite) IsDev() bool
func (v *Vite) Tags(entries ...string) (template.HTML, error)
func (v *Vite) Asset(path string) (string, error)
```

### Dev mode vs build mode

Dev mode is on when `DevURL` is set **or** the hot file exists, unless `DisableDev` is true. The hot file is always read **from the working directory on disk** (`os.ReadFile`), even when `FS` is set.

> **Production: set `DisableDev: true`.** Otherwise a stale `public/hot` next to the binary (left by a SIGKILLed Vite, copied by a deploy script, or written by anyone with write access to that directory) switches every page to load its JavaScript from the URL inside that file. With `DisableDev` the manifest is always used and the hot file is never read.

- **Dev:** `Tags` emits `<script type="module" src="<dev>/@vite/client">` plus one tag per entry (`<link rel="stylesheet">` for `.css`, module script otherwise). `Version()` is `""`.
- **Build:** `Tags` reads the manifest (from `FS` if set, else disk; cached after first read) and emits, in order: all CSS `<link rel="stylesheet">` (entry CSS + CSS of imported chunks, de-duplicated), `<link rel="modulepreload">` for imported chunks, and `<script type="module">` for each entry. URLs are prefixed with `BaseURL`.

Errors:
- manifest missing → `ginertia: vite manifest not found (run `vite build` or start `vite`)`
- entry not in manifest → `ginertia: "<entry>" is not in the vite manifest (add it to build.rollupOptions.input)`

Serve the build directory at `BaseURL`:

```go
build, _ := fs.Sub(files, "public/build")
r.StaticFS("/build", http.FS(build))
```

### `vite-plugin-ginertia.js` options

```js
ginertia({
  input: 'frontend/app.js',   // client entry (string or array)
  ssr: 'frontend/ssr.js',     // SSR entry, used by `vite build --ssr`
  publicDir: 'public',
  buildDir: 'build',          // → public/build, base /build/
  hotFile: 'public/hot',      // default publicDir/hot
  ssrOutDir: 'bootstrap/ssr',
})
```

What it configures:
- `base`: `/build/` for builds, `''` in dev.
- `publicDir: false` (Vite's public dir is disabled; serve static files from Gin).
- Dev server: `strictPort`, `cors: true`, `origin: http://localhost:<port>` so assets load from Vite, not Gin.
- Build: `manifest: true`, output `public/build`; SSR build: no manifest, output `bootstrap/ssr`.
- Writes the dev server URL to `public/hot` when Vite starts listening; deletes it on exit (SIGINT/SIGTERM/SIGHUP).

If Vite is killed with SIGKILL, `public/hot` may remain and the Go server will keep using dev mode. Delete it manually (in production `DisableDev: true` makes this harmless).

---

## 15. Server-side rendering (SSR)

```go
type SSR struct {
	URL     string                          // default "http://127.0.0.1:13714"
	Timeout time.Duration                   // default 2s
	Client  *http.Client                    // default http.DefaultClient
	OnError func(c *gin.Context, err error) // e.g. log it
}
```

```go
cfg.SSR = &ginertia.SSR{OnError: func(c *gin.Context, err error) { log.Println("ssr:", err) }}
```

On every **non-Inertia** (HTML) response, the engine POSTs the page JSON to `<URL>/render` with `Content-Type: application/json` and expects `200` with `{"head": [string], "body": string}`.

- Success: `.inertia` = `body`, `.inertiaHead` = `head` joined with `\n`.
- Any failure (connection error, timeout, non-200, invalid JSON, empty body): `OnError` is called if set, and the page falls back to normal client-side rendering. A dead or hanging SSR server never breaks the site; worst case adds `Timeout` latency.
- Inertia JSON responses never call SSR.

SSR entry (`frontend/ssr.js`, Svelte):

```js
import { createInertiaApp } from '@inertiajs/svelte'
import createServer from '@inertiajs/svelte/server'
import { render } from 'svelte/server'
import Layout from './Layout.svelte'

createServer((page) =>
  createInertiaApp({
    page,
    resolve: (name) => import.meta.glob('./Pages/**/*.svelte', { eager: true })[`./Pages/${name}.svelte`],
    layout: () => Layout,
    setup: ({ App, props }) => render(App, { props }),
  }),
)
```

Run:

```bash
npm run build                 # client + SSR bundles
node bootstrap/ssr/ssr.js &   # listens on 13714
./app                         # with cfg.SSR set
```

In the root template, render a fallback `<title>` only when there is no SSR head: `{{ if not .inertiaHead }}<title>…</title>{{ end }}`.

---

## 16. CSRF protection

```go
type CSRFConfig struct {
	Secret         []byte                    // required (panics if empty); same on every instance
	CookieName     string                    // default "XSRF-TOKEN"
	HeaderName     string                    // default "X-XSRF-TOKEN"
	FormField      string                    // default "_token" (used when the header is absent)
	Secure         bool                      // cookie Secure flag; auto-on for TLS requests
	TrustedOrigins []string                  // extra origins allowed to POST, "https://admin.example.com"
	Skip           func(c *gin.Context) bool // bypass all checks (e.g. webhooks)
	OnFailure      func(c *gin.Context)      // default: 419 (Laravel's "Page Expired")
}

func CSRF(cfg CSRFConfig) gin.HandlerFunc
```

Unsafe requests (anything except `GET`, `HEAD`, `OPTIONS`, `TRACE`) must pass **two independent checks**.

**1. Origin check** (same logic as Go 1.25's `http.CrossOriginProtection`):

- `Sec-Fetch-Site: same-origin` or `none` → passes.
- `Sec-Fetch-Site: same-site` or `cross-site` → rejected, unless `Origin` is in `TrustedOrigins`. `same-site` covers sibling subdomains, which matters because a subdomain can plant cookies for your domain ("cookie tossing").
- No `Sec-Fetch-Site` (older browsers): no `Origin` → passes (non-browser client; the token check still applies); `Origin` host equals the request `Host` → passes; otherwise only a trusted origin passes. `Origin: null` is rejected.

`TrustedOrigins` entries must be `scheme://host[:port]` with no path (invalid entries panic at startup). Typical uses: a frontend served from another domain, or a reverse proxy that rewrites `Host` so it no longer matches the browser's `Origin` (configure the proxy to preserve `Host` when possible).

**2. Signed double-submit token:**

- Token = `nonce.HMAC-SHA256(secret, "ginertia-csrf:"+nonce)`, nonce = 32 random bytes (base64url).
- Safe methods (`GET`, `HEAD`, `OPTIONS`, `TRACE`): if no valid token cookie exists, a new one is issued. The cookie is readable by JS (`HttpOnly: false`, `SameSite=Lax`) on purpose.
- Unsafe methods: the token from the header (or form field `_token`) must equal the cookie and the cookie must have a valid signature. On failure → `OnFailure` (default `419`), and a fresh cookie is issued if the old one was missing/invalid so the next attempt succeeds.
- No frontend code needed: the Inertia client (axios-style XSRF handling) copies `XSRF-TOKEN` into `X-XSRF-TOKEN` automatically.

Handle 419 on the client, e.g. reload the page or show "session expired":

```js
router.on('httpException', (event) => {
  if (event.detail.response.status === 419) { /* reload or show message */ }
})
```

Plain HTML forms (non-Inertia) must include the token in a `_token` field.

When the header is absent, the middleware reads `_token` with `c.PostForm`, which parses the request body (multipart: up to Gin's `MaxMultipartMemory` in RAM, the rest in temp files) before the token is checked. Cross-origin browser requests are rejected earlier by the origin check, but a non-browser client can still make the server parse a large body. Put a body size limit in front of `CSRF` on routes that do not need large uploads:

```go
r.Use(func(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20) // 1 MB
	c.Next()
})
r.Use(ginertia.CSRF(...))
```

`Secure` is only switched on automatically for direct TLS connections. Behind a TLS-terminating proxy (nginx, Cloudflare, a load balancer) set `Secure: true` explicitly — the same applies to `CookieFlashStore.Secure`.

The token is not bound to a user session. On its own, a double-submit token can be defeated by a sibling subdomain that plants its own validly signed `XSRF-TOKEN` cookie; the origin check above closes that hole for every modern browser (all of them send `Sec-Fetch-Site`). `security_test.go` and `example/e2e/csrf.py` reproduce that attack and assert it gets 419.

Excluding webhooks:

```go
r.Use(ginertia.CSRF(ginertia.CSRFConfig{
	Secret: key,
	Skip:   func(c *gin.Context) bool { return strings.HasPrefix(c.Request.URL.Path, "/webhooks/") },
}))
```

---

## 17. Flash stores

```go
type FlashData struct {
	Errors       map[string]any `json:"e,omitempty"`
	Flash        map[string]any `json:"f,omitempty"`
	ClearHistory bool           `json:"c,omitempty"`
}

type FlashStore interface {
	// Read returns the stored data and removes it from the store.
	Read(c *gin.Context) (*FlashData, error)
	// Write stores data for the next request; nil (or empty data) clears it.
	Write(c *gin.Context, data *FlashData) error
}
```

### `CookieFlashStore` (default)

```go
type CookieFlashStore struct {
	Name   string // default "ginertia_flash"
	Secret []byte // HMAC key; if empty, a random key is generated at startup
	Path   string // default "/"
	Domain string
	Secure bool          // auto-on for TLS requests; set true behind a TLS proxy
	MaxAge time.Duration // cookie lifetime, default 5 minutes
}
```

- Cookie value: `base64url(JSON{d: data, t: issued-at}) + "." + base64url(HMAC-SHA256)`. `HttpOnly`, `SameSite=Lax`, `Max-Age` = `MaxAge`.
- The issue time is inside the signed payload: cookies older than `MaxAge` (or dated more than a minute in the future) are rejected. A captured cookie therefore cannot be replayed later, nor planted into another browser by a sibling subdomain ("cookie tossing") outside that window. Cookies written by v0.1.x (no issue time) are rejected once after upgrading.
- Strongest scoping: `Name: "__Host-flash", Secure: true` and no `Domain` — browsers then refuse the cookie over HTTP and from subdomains.
- Tampered or unsigned cookies are rejected (read returns an error; the middleware ignores it).
- Without `Secret`, data is still tamper-proof but does not survive a restart and does not work across multiple instances. **Always set `Secret` in production.**
- Values over ~4000 bytes fail with `ginertia: flash data too large for a cookie, use a server-side FlashStore` (recorded via `c.Error`; the data is dropped). Use a server-side store for large error maps.
- Repeated writes in one response replace the previous `Set-Cookie` for the same cookie (last write wins).

### Custom store example (gin-contrib/sessions)

```go
type SessionFlash struct{}

const flashKey = "_ginertia_flash"

func (SessionFlash) Read(c *gin.Context) (*ginertia.FlashData, error) {
	s := sessions.Default(c)
	raw, ok := s.Get(flashKey).([]byte)
	if !ok {
		return nil, nil
	}
	s.Delete(flashKey)
	if err := s.Save(); err != nil {
		return nil, err
	}
	var d ginertia.FlashData
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (SessionFlash) Write(c *gin.Context, d *ginertia.FlashData) error {
	s := sessions.Default(c)
	if d == nil || (len(d.Errors) == 0 && len(d.Flash) == 0 && !d.ClearHistory) {
		s.Delete(flashKey)
	} else {
		b, err := json.Marshal(d)
		if err != nil {
			return err
		}
		s.Set(flashKey, b)
	}
	return s.Save()
}

// Config{Flash: SessionFlash{}} — and register sessions.Sessions(...) BEFORE inertia.Middleware().
```

`Write` may be called several times per request (each `Flash`/`WithErrors`/`ClearHistory` call persists immediately, and `Render` clears consumed data with `Write(c, nil)`), so it must be idempotent.

---

## 18. Wire protocol reference

### Request headers read by ginertia

| Header | Meaning |
|---|---|
| `X-Inertia: true` | Inertia visit → JSON response |
| `X-Inertia-Version` | client asset version; mismatch on GET → 409 |
| `X-Inertia-Partial-Component` | component for a partial reload; must equal the rendered component |
| `X-Inertia-Partial-Data` | comma-separated `only` keys (top-level only) |
| `X-Inertia-Partial-Except` | comma-separated `except` keys |
| `X-Inertia-Reset` | comma-separated keys whose merge metadata is dropped (client replaces instead of merging) |
| `X-Inertia-Except-Once-Props` | once-prop keys the client already holds |
| `X-Inertia-Error-Bag` | nests `WithErrors` output under this bag name |
| `X-Inertia-Infinite-Scroll-Merge-Intent` | `prepend` → scroll items go to `prependProps`; otherwise `mergeProps` |
| `Purpose: prefetch` | do not consume flash data |
| `Referer` | used by `Back` |
| `X-XSRF-TOKEN` / form `_token` | CSRF token (CSRF middleware) |

### Response headers / status codes

| Situation | Status | Headers |
|---|---|---|
| Inertia render | 200 | `X-Inertia: true`, `Vary: X-Inertia`, `Content-Type: application/json; charset=utf-8` |
| HTML render | 200 | `Vary: X-Inertia`, `Content-Type: text/html; charset=utf-8` |
| `Redirect` on GET | 302 | `Location` |
| `Redirect`/`Back` on non-GET, or 302 after PUT/PATCH/DELETE (Inertia) | 303 | `Location` |
| Asset version mismatch | 409 | `X-Inertia-Location: <relative request URI>` |
| `Location()` on Inertia request | 409 | `X-Inertia-Location: <target>` |
| `Location()` on normal request | 302 | `Location` |
| CSRF failure | 419 | — |
| Resolver/marshal/template error, unsafe `Location` | 500 | empty body |

### HTML payload

```html
<script data-page="app" type="application/json">{…page JSON…}</script><div id="app"></div>
```

Every `<` in the JSON is replaced with `\u003c` before embedding, regardless of `Config.Marshal`, so prop content can never close the script element. This is regression-tested and fuzzed with both `encoding/json` and an encoder that does not escape HTML.

### Exported header constants

```go
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
HeaderMergeIntent      = "X-Inertia-Infinite-Scroll-Merge-Intent"
```

---

## 19. Recipes

### 19.1 Authentication

```go
// Share the current user with every page (register after inertia.Middleware()).
func shareUser(c *gin.Context) {
	if u, ok := currentUser(c); ok {
		ginertia.Share(c, "auth", gin.H{"user": gin.H{"id": u.ID, "name": u.Name}}) // DTO, not the DB model
	} else {
		ginertia.Share(c, "auth", gin.H{"user": nil})
	}
	c.Next()
}

// Protect a route group.
func requireAuth(c *gin.Context) {
	if _, ok := currentUser(c); !ok {
		ginertia.Redirect(c, "/login") // works for full loads and Inertia visits
		return
	}
	ginertia.EncryptHistory(c, true)
	c.Next()
}

app := r.Group("/", requireAuth)
app.GET("/", home)
app.POST("/logout", logout) // calls ginertia.ClearHistory(c) then Redirect("/login")
```

```svelte
<!-- Layout.svelte -->
<script>
  import { Link, page } from '@inertiajs/svelte'
</script>
{#if page.props.auth.user}
  <span>{page.props.auth.user.name}</span>
  <Link href="/logout" method="post" as="button">Log out</Link>
{/if}
```

### 19.2 "Load more" with `Merge`

```go
users, hasMore := store.Page(page, 8)
ginertia.Render(c, "Users/Index", gin.H{
	"users":   ginertia.Merge(users).MatchOn("id"),
	"page":    page,
	"hasMore": hasMore,
})
```

```js
router.reload({ data: { page: page + 1 }, only: ['users', 'page', 'hasMore'] })
```

### 19.3 Infinite scroll

```go
const perPage = 15
page := max(atoi(c.DefaultQuery("page", "1")), 1)
posts := repo.Posts(offset(page, perPage), perPage+1) // one extra row
hasMore := len(posts) > perPage
if hasMore { posts = posts[:perPage] }

ginertia.Render(c, "Feed", gin.H{
	"posts": ginertia.ScrollPage(posts, page, hasMore).MatchOn("id"),
})
```

```svelte
<script>
  import { InfiniteScroll } from '@inertiajs/svelte'
  let { posts } = $props()
</script>
<InfiniteScroll data="posts" buffer={200}>
  {#each posts.data as post (post.id)}<article>{post.title}</article>{/each}
  {#snippet loading()}Loading…{/snippet}
</InfiniteScroll>
```

Cursor pagination:

```go
ginertia.Scroll(gin.H{"data": items}, ginertia.ScrollMeta{
	PageName:    "cursor",
	CurrentPage: cursor,
	NextPage:    nextCursor, // nil when done
})
```

### 19.4 Deferred props

```go
ginertia.Render(c, "Dashboard", gin.H{
	"summary": summary,
	"stats":   ginertia.Defer(func() any { return slowStats() }),             // group "default"
	"chart":   ginertia.Defer(func() any { return slowChart() }, "charts"),   // separate request
})
```

```svelte
<script>
  import { Deferred } from '@inertiajs/svelte'
  let { stats } = $props()
</script>
<Deferred data="stats">
  {#snippet fallback()}<div class="skeleton"></div>{/snippet}
  <b>{stats.total}</b>
</Deferred>
```

### 19.5 Partial reloads and polling

```go
ginertia.Render(c, "Home", gin.H{
	"serverTime": func() string { return time.Now().Format("15:04:05") }, // only runs when requested
	"heavy":      ginertia.Optional(func() any { return expensive() }),   // never on first load
})
```

```js
router.reload({ only: ['serverTime'] })
const poll = usePoll(2000, { only: ['serverTime'] })
```

### 19.6 Once props

```go
"countries": ginertia.Once(func() any { return repo.Countries() }).Until(24 * time.Hour),
```

The client keeps the value across visits and tells the server via `X-Inertia-Except-Once-Props`; the resolver is skipped until it expires or is explicitly requested with `only`.

### 19.7 External redirect / OAuth

```go
r.GET("/auth/github", func(c *gin.Context) {
	ginertia.Location(c, oauthCfg.AuthCodeURL(state))
})
```

### 19.8 Custom JSON encoder

```go
import "github.com/bytedance/sonic"

ginertia.Config{Marshal: sonic.Marshal}
```

HTML payload safety does not depend on the encoder: ginertia replaces every `<` with `\u003c` before embedding the JSON in the `<script>` tag, so encoders that don't escape HTML (such as `sonic.Marshal`) are safe.

### 19.9 Custom error handler

```go
ginertia.Config{
	ErrorHandler: func(c *gin.Context, err error) {
		slog.Error("inertia render", "err", err, "path", c.Request.URL.Path)
		c.AbortWithStatus(http.StatusInternalServerError)
	},
}
```

Don't write `err.Error()` to the response body; it may contain internal details.

### 19.10 Per-page `<title>` without SSR

Either use `<svelte:head><title>…</title></svelte:head>` in components (updates on client navigation), or `ViewData(c, "title", …)` (first load only). Using both is fine.

---

## 20. Testing your handlers

```go
func TestUsersIndex(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := ginertia.MustNew(ginertia.Config{
		RootView: "app.html",
		RootFS:   fstest.MapFS{"app.html": {Data: []byte(`<html><head>{{ .inertiaHead }}</head><body>{{ .inertia }}</body></html>`)}},
		Version:  "test",
	})
	r := gin.New()
	r.Use(e.Middleware())
	r.GET("/users", usersIndex)

	req := httptest.NewRequest("GET", "/users", nil)
	req.Header.Set(ginertia.HeaderInertia, "true")
	req.Header.Set(ginertia.HeaderVersion, "test") // must match, or you get 409
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var page ginertia.Page
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Component != "Users/Index" { t.Fatalf("component %q", page.Component) }
}
```

Tips:
- Set a fixed `Version` so the version header can match.
- Partial reload: add `X-Inertia-Partial-Component` and `X-Inertia-Partial-Data`.
- To test errors/flash across a redirect, copy the `Set-Cookie` from the first response into the second request.
- `Engine.BuildPage` can be called directly inside a handler under test to inspect the page without HTTP.

Library tests:

```bash
go test -race ./...
go test -run=^$ -fuzz=FuzzPageScript -fuzztime=60s .
go test -run=^$ -fuzz=FuzzFlashCookie -fuzztime=60s .
```

Browser end-to-end security checks (Playwright, against the example app built with `npm run build` and started with `APP_ENV=production APP_KEY=x ./app`):

```bash
python3 example/e2e/xss.py CSR      # XSS payloads through props (run again with SSR=1 and the SSR server)
python3 example/e2e/csrf.py         # CSRF from a sibling subdomain with a planted signed token
python3 example/e2e/e2e_all.py      # full feature run
```

---

## 21. Production deployment

1. `npm run build` **before** `go build` (embedding `public/` requires the build output to exist).
2. `go build` with `//go:embed views all:public` and `RootFS`/`Vite.FS` pointing at the embed FS. The `all:` prefix is required so `public/build/.vite/manifest.json` (a dot-directory) is embedded.
3. Set `Vite.DisableDev: true` so a stale or planted `public/hot` can never switch the site to a dev server URL.
4. Set `APP_KEY` (or your equivalent) to the same random secret on every instance; use it for `CookieFlashStore.Secret` and `CSRFConfig.Secret`.
5. Serve over HTTPS (cookies get `Secure` automatically on TLS; behind a TLS-terminating proxy, set `Secure: true` explicitly on both `CSRFConfig` and `CookieFlashStore`).
6. Deploy all instances with the same build. Mixed builds behind a load balancer produce different versions → repeated 409 full reloads.
7. Optional SSR: run `node bootstrap/ssr/ssr.js` as a sidecar/service and set `Config.SSR`.
8. Set `ReloadRootView: false` and `gin.SetMode(gin.ReleaseMode)`.

Performance reference (from `_bench/`, 2 vCPU VM, mixed traffic): ~9,500 req/s, p99 ~22 ms; post-GC heap flat over 1.7M requests; 0 data races under `-race`.

---

## 22. Security model

Handled by the library (regression tests in `security_test.go`):

| Risk | Mitigation |
|---|---|
| XSS through props in the HTML payload | every `<` escaped as `\u003c` regardless of `Config.Marshal`; fuzz-tested with an HTML-escaping and a non-escaping encoder |
| Open redirect via `Referer` in `Back` | same-host or path-only referers, reduced to path; `//` and `/\` rejected |
| Host header injection on 409 | `X-Inertia-Location` is a relative URI |
| Protocol-relative URL from a `//evil.example` request path | leading slashes are collapsed in the 409 `X-Inertia-Location` and in `page.url` |
| CSRF, including from sibling subdomains (cookie tossing) | `CSRF` middleware: `Sec-Fetch-Site` / `Origin` check plus HMAC-signed double-submit token; 419 on failure |
| Forged flash/errors | HMAC-signed cookie; random key if `Secret` unset |
| Replayed / tossed flash cookie | signed issue time, rejected after `CookieFlashStore.MaxAge` (default 5 min) |
| Scripts loaded from a stale/planted `public/hot` | `Vite.DisableDev` ignores the hot file and `DevURL` |
| `javascript:` URLs in `Location` | only empty/`http`/`https` schemes allowed |
| Internal error leakage | parser errors replaced by a generic message; resolver errors → empty 500 |
| Clobbering `Vary` set by other middleware | `Vary` is appended |

Your responsibility:

- **All props reach the browser.** Use DTOs or `json:"-"` for sensitive fields.
- `Optional`/`Defer`/`Once` are delivery mechanisms, not authorization.
- Authorize every handler, including those only reached via partial reloads.
- Use a strong, shared secret; HTTPS in production (`Secure: true` behind a TLS proxy).
- Never pass user input to `Location`/`Redirect` without `IsLocalURL` (or an allowlist).
- Set `Vite.DisableDev: true` in production.
- Limit request body size in front of `CSRF` where large uploads are not expected.
- On logout: `ClearHistory(c)` server side and `router.flushAll()` client side.

---

## 23. Limitations

- **Precognition** (live validation while typing) is not implemented.
- **Dot notation in `only` / `except`** (`only: ['users.data']`) is not supported; only top-level prop keys are matched.
- The Vite plugin is a file to copy, not an npm package.
- The asset version (manifest md5) and the manifest itself are cached for the process lifetime.
- `CookieFlashStore` is limited to ~4 KB of encoded data.

---

## 24. Troubleshooting

| Symptom | Cause / fix |
|---|---|
| panic `Middleware() is not registered on this route` | `r.Use(inertia.Middleware())` missing or registered after the route/group |
| `vite manifest not found` | run `npm run build`, or start `npm run dev`; check `ManifestPath`/`FS` |
| `"x" is not in the vite manifest` | entry passed to `{{ vite }}` must match `input` in the Vite plugin exactly |
| Assets 404 in production | missing `r.StaticFS("/build", …)` or `BaseURL` mismatch |
| No HMR in dev | Vite not running, or Go not started from the directory containing `public/hot` |
| Production serving dev-server URLs | stale `public/hot` next to the binary; delete it |
| Every navigation does a full reload | version mismatch: different builds across instances, or a changing `VersionFunc` |
| Flash/errors lost after redirect | `Secret` differs across instances or is unset with multiple instances; data > 4 KB; or the redirect target loaded later than `CookieFlashStore.MaxAge` (server clocks out of sync?) |
| Production page loads scripts from `localhost:5173` | stale `public/hot`; set `Vite.DisableDev: true` |
| `419` on every POST | `CSRFConfig.Secret` differs across instances, or a non-Inertia client is not sending `X-XSRF-TOKEN`/`_token` |
| `419` only in production behind a proxy | the proxy rewrites `Host`, so the browser's `Origin` no longer matches; preserve `Host` in the proxy or add the public origin to `CSRFConfig.TrustedOrigins` |
| `419` from a separate frontend domain | add that origin to `CSRFConfig.TrustedOrigins` |
| Props missing after `router.reload({ only })` | prop key not in `only`, or nested key used (`a.b`); use top-level keys |
| Partial reload returns all props | component name in `Render` differs from the page's current component |
| Resolver func causes 500 | signature not supported (see §8.1) or it returned an error; check `c.Errors` / `ErrorHandler` |
| Validation error keys don't match form fields | add `json`/`form` tags matching the client form keys |
| `errors` key `"_"` | request body could not be parsed (wrong `Content-Type`, malformed JSON) |
| Back button shows private page after logout | add `router.flushAll()` on `clearHistory` and call `ClearHistory(c)` |

---

## 25. Complete exported API

```go
// Engine
type Config struct{ … }                       // §5
type Engine struct{ … }
func New(cfg Config) (*Engine, error)
func MustNew(cfg Config) *Engine
func (e *Engine) Middleware() gin.HandlerFunc
func (e *Engine) Share(key string, value any)
func (e *Engine) Version() string
func (e *Engine) Render(c *gin.Context, component string, props map[string]any)
func (e *Engine) BuildPage(c *gin.Context, component string, props map[string]any) (*Page, error)

// Request helpers
func IsInertia(c *gin.Context) bool
func Render(c *gin.Context, component string, props map[string]any)
func Share(c *gin.Context, key string, value any)
func ViewData(c *gin.Context, key string, value any)
func EncryptHistory(c *gin.Context, encrypt bool)
func ClearHistory(c *gin.Context)

// Redirects
func Redirect(c *gin.Context, location string)
func Back(c *gin.Context, fallback ...string)
func Location(c *gin.Context, target string)

// Errors & flash
func WithErrors[V any](c *gin.Context, errs map[string]V)
func Flash(c *gin.Context, key string, value any)
func ValidationErrors(err error, obj any, msg ...MessageFunc) map[string]string
type MessageFunc func(fe validator.FieldError, field string) string
var DefaultMessage MessageFunc

// Props
type Props = map[string]any
func Optional(resolver any) OptionalProp
func Lazy(resolver any) OptionalProp
func Always(value any) AlwaysProp
func Defer(resolver any, group ...string) DeferProp
func (DeferProp) Merge() DeferProp
func (DeferProp) DeepMerge() DeferProp
func Merge(value any) MergeProp
func DeepMerge(value any) MergeProp
func (MergeProp) Prepend() MergeProp
func (MergeProp) MatchOn(fields ...string) MergeProp
func Once(resolver any) OnceProp
func (OnceProp) Until(ttl time.Duration) OnceProp
func Scroll(value any, meta ScrollMeta) ScrollProp
func ScrollPage(items any, page int, hasMore bool) ScrollProp
func (ScrollProp) Wrapper(key string) ScrollProp
func (ScrollProp) PageName(name string) ScrollProp
func (ScrollProp) MatchOn(fields ...string) ScrollProp
type ScrollMeta struct{ PageName string; PreviousPage, NextPage, CurrentPage any }

// Page object
type Page struct{ … }                         // §7
type OnceMeta struct{ Prop string; ExpiresAt *int64 }
type ScrollMetaOut struct{ ScrollMeta; Reset bool }

// Vite / SSR
type Vite struct{ DisableDev bool; HotFile, DevURL, BaseURL, ManifestPath string; FS fs.FS }
func (*Vite) IsDev() bool
func (*Vite) Tags(entries ...string) (template.HTML, error)
func (*Vite) Asset(path string) (string, error)
type SSR struct{ URL string; Timeout time.Duration; Client *http.Client; OnError func(*gin.Context, error) }

// Flash storage
type FlashData struct{ Errors, Flash map[string]any; ClearHistory bool }
type FlashStore interface {
	Read(c *gin.Context) (*FlashData, error)
	Write(c *gin.Context, data *FlashData) error
}
type CookieFlashStore struct{ Name string; Secret []byte; Path, Domain string; Secure bool; MaxAge time.Duration }

// CSRF
type CSRFConfig struct{ … }                   // §16
func CSRF(cfg CSRFConfig) gin.HandlerFunc

// Header constants: §18
```

---

## 26. Integration checklist

Use this list when adding ginertia to an existing Gin app (or when an AI agent scaffolds one):

- [ ] `go get` the module; install Vite, the framework plugin and the matching `@inertiajs/*` client.
- [ ] Copy `vite-plugin-ginertia.js`; configure `input` (and `ssr` if used).
- [ ] Create `views/app.html` with `{{ vite "<entry>" }}`, `{{ .inertiaHead }}` and `{{ .inertia }}`.
- [ ] Create the client entry with a `resolve` that maps `"Dir/Name"` → `./Pages/Dir/Name.<ext>`.
- [ ] Create `ginertia.MustNew(Config{RootView, RootFS, Vite, Flash: &CookieFlashStore{Secret}})`.
- [ ] Serve `public/build` at `/build`.
- [ ] Register `ginertia.CSRF(...)` then `inertia.Middleware()` before routes.
- [ ] Share global props with `Engine.Share`; per-request props (auth user) via middleware calling `ginertia.Share(c, …)`.
- [ ] In handlers: `Render` for pages; `WithErrors` + `Back` on validation failure; `Flash` + `Redirect` on success.
- [ ] Add `json` + `form` tags to input structs so error keys match client form keys.
- [ ] Map DB models to DTOs before passing them as props.
- [ ] Logout: `ClearHistory(c)` + client `router.flushAll()` on `clearHistory`.
- [ ] Production: build assets before `go build`, embed `all:public`, shared secret, HTTPS, same build on every instance.
- [ ] Production: `Vite{DisableDev: true}`, `Secure: true` on `CSRFConfig` + `CookieFlashStore` behind a TLS proxy, body size limit before `CSRF`.
- [ ] Redirects to user-supplied URLs go through `IsLocalURL`.
