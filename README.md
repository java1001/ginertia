# ginertia

**Inertia.js v3 server adapter for Gin.** Build modern single-page apps with Svelte, Vue or React while keeping routing, controllers and data loading in Go: no REST API, no client-side router.

📖 Documentation: [DOCS.md](https://github.com/java1001/ginertia/blob/main/DOCS.md)

## Description

ginertia brings the Laravel + Inertia workflow to Go. Your Gin handlers render a page component and pass Go values straight in as its props, just like `Inertia::render` in Laravel. The first visit returns a full HTML page; every navigation after that is a small JSON response that the Inertia client swaps in without a page reload.

You get the feel of an SPA (instant navigation, client-side state, HMR) with the simplicity of a server-rendered app: one router, one place for auth and validation, and no API layer to design, version and keep in sync.

```go
r.GET("/users", func(c *gin.Context) {
    ginertia.Render(c, "Users/Index", gin.H{"users": users})
})
```

```svelte
<!-- frontend/Pages/Users/Index.svelte -->
<script>
  let { users } = $props()
</script>
{#each users as u}<p>{u.name}</p>{/each}
```

**Highlights**

- Full Inertia v3 protocol: partial reloads, deferred / optional / always / merge / once props, infinite scroll, asset versioning, error bags, history encryption.
- Laravel-style helpers: `Share`, `Flash`, `WithErrors`, `Back`, `Redirect`, `Location`.
- Vite integration with HMR in development and manifest-based tags in production.
- Optional server-side rendering with automatic fallback to client rendering.
- Built-in CSRF middleware and HMAC-signed flash cookies.
- Validation errors from `go-playground/validator` mapped to form field names.
- Ships as a single binary: views and assets can be embedded with `embed.FS`.
- Frontend-agnostic: works with `@inertiajs/svelte`, `@inertiajs/vue3` and `@inertiajs/react`.

Tested with `@inertiajs/svelte` 3.8, Svelte 5, Vite 8, Gin 1.11 and Go 1.24.

---

## Installation

```bash
go get github.com/java1001/ginertia@latest
```

> Change the module path in `go.mod` to your own repository before publishing. If you build straight from a source archive, run `go mod tidy` once so Go fetches the checksums.

Frontend:

```bash
npm i -D vite @sveltejs/vite-plugin-svelte svelte @inertiajs/svelte
```

Copy `example/vite-plugin-ginertia.js` into your project. It works like `laravel-vite-plugin`: while `vite` is running it writes `public/hot` so the Go server switches to the dev server (HMR), and `vite build` outputs to `public/build` with a manifest.

## Setup

```go
prod := os.Getenv("APP_ENV") == "production"

inertia := ginertia.MustNew(ginertia.Config{
    RootView: "views/app.html",
    RootFS:   files,                    // os.DirFS(".") in dev, embed.FS in production
    Vite:     &ginertia.Vite{FS: files, DisableDev: prod}, // prod: never read public/hot
    Flash:    &ginertia.CookieFlashStore{Secret: []byte(os.Getenv("APP_KEY")), Secure: prod},
})
inertia.Share("appName", "My App")      // shared with every page

r := gin.Default()
r.StaticFS("/build", http.FS(buildFS))
r.Use(inertia.Middleware())
```

`views/app.html`:

```html
<!doctype html>
<html>
<head>
  {{ vite "frontend/app.js" }}
  {{ .inertiaHead }}
</head>
<body>{{ .inertia }}</body>
</html>
```

`frontend/app.js`:

```js
import { createInertiaApp } from '@inertiajs/svelte'

createInertiaApp({
  resolve: (name) => import.meta.glob('./Pages/**/*.svelte', { eager: true })[`./Pages/${name}.svelte`],
})
```

## API: Laravel vs. ginertia

| Laravel | ginertia |
|---|---|
| `Inertia::render('Users/Index', [...])` | `ginertia.Render(c, "Users/Index", gin.H{...})` |
| `Inertia::share('key', $v)` | `engine.Share("key", v)` (global) · `ginertia.Share(c, "key", v)` (per request) |
| `fn () => ...` (lazy evaluation) | `func() any`, `func(*gin.Context) (any, error)`, or a typed func such as `func() []User` |
| `Inertia::optional(fn)` | `ginertia.Optional(fn)` (alias `Lazy`) |
| `Inertia::always($v)` | `ginertia.Always(v)` |
| `Inertia::defer(fn, 'group')` | `ginertia.Defer(fn, "group")` · `.Merge()` · `.DeepMerge()` |
| `Inertia::merge($v)` | `ginertia.Merge(v)` · `.Prepend()` · `.MatchOn("id")` · `ginertia.DeepMerge(v)` |
| `Inertia::once(fn)` | `ginertia.Once(fn).Until(time.Hour)` |
| `Inertia::scroll($paginator)` | `ginertia.ScrollPage(items, page, hasMore)` · `ginertia.Scroll(v, ScrollMeta{...})` for cursors |
| `back()->withErrors($e)` | `ginertia.WithErrors(c, errs); ginertia.Back(c)` |
| `$request->validate()` | `ginertia.ValidationErrors(c.ShouldBindJSON(&in), in)` |
| `session()->flash('success', ...)` | `ginertia.Flash(c, "success", "...")` → `page.flash.success` |
| `redirect('/users')` | `ginertia.Redirect(c, "/users")` (or `c.Redirect(302)`: the middleware upgrades it to 303) |
| `Inertia::location($url)` | `ginertia.Location(c, url)` |
| `Inertia::encryptHistory()` / `clearHistory()` | `ginertia.EncryptHistory(c, true)` / `ginertia.ClearHistory(c)` |
| `view()->with(...)` for the root template | `ginertia.ViewData(c, "title", "...")` |
| `VerifyCsrfToken` middleware | `r.Use(ginertia.CSRF(ginertia.CSRFConfig{Secret: key}))` |

### Example: form with validation

```go
type UserForm struct {
    Name  string `json:"name"  binding:"required,min=2"`
    Email string `json:"email" binding:"required,email"`
}

func store(c *gin.Context) {
    var in UserForm
    if err := c.ShouldBindJSON(&in); err != nil {
        ginertia.WithErrors(c, ginertia.ValidationErrors(err, in)) // {"email": "The email must be..."}
        ginertia.Back(c)
        return
    }
    // ... db.Create(...)
    ginertia.Flash(c, "success", "Created.")
    ginertia.Redirect(c, "/users")
}
```

```svelte
<script>
  import { useForm } from '@inertiajs/svelte'
  const form = useForm({ name: '', email: '' })
</script>

<form onsubmit={(e) => { e.preventDefault(); form.post('/users') }}>
  <input bind:value={form.email} />
  {#if form.errors.email}<small>{form.errors.email}</small>{/if}
</form>
```

Error keys follow the `form` / `json` struct tags and support nested fields (`address.city`) and slices (`tags.0.label`). To customize or translate messages, assign `ginertia.DefaultMessage = func(fe validator.FieldError, field string) string { ... }`.

## Protocol support

- The first visit returns HTML with `<script data-page type="application/json">` (the v3 format). Subsequent visits return JSON with `X-Inertia: true` and `Vary: X-Inertia`.
- Asset versioning: on a version mismatch the server responds 409 with `X-Inertia-Location` and the client does a full reload. The default version is the md5 of the Vite manifest. Flash data survives that reload.
- Partial reloads (`only` / `except`), applied only when the component matches.
- Deferred props by group; merge / prepend / deep merge / matchOn; the `X-Inertia-Reset` header.
- Once props (`X-Inertia-Except-Once-Props`, `expiresAt`).
- `sharedProps`, `page.flash`, error bags (`X-Inertia-Error-Bag`).
- 302 → 303 for PUT / PATCH / DELETE.
- History encryption.
- SSR through the Inertia SSR server (`/render`). If the SSR server is down, the page falls back to client-side rendering.
- `<` is escaped in the page JSON, even when you swap `Marshal` for sonic or goccy/go-json.
- Infinite scroll (`scrollProps` + `X-Inertia-Infinite-Scroll-Merge-Intent`): scrolling down appends; opening `?page=4` and scrolling up prepends.
- Prefetch requests (`Purpose: prefetch`) do not consume flash data.

**Not yet supported:** Precognition (live validation while typing) and dot notation in `only` (`only: ['users.data']`). Only top-level keys are supported for now.

### Infinite scroll

```go
// fetch perPage+1 rows to know whether another page exists
ginertia.Render(c, "Feed", gin.H{
    "posts": ginertia.ScrollPage(posts, page, hasMore).MatchOn("id"),
})
```

```svelte
<InfiniteScroll data="posts">
  {#each posts.data as post (post.id)} ... {/each}
</InfiniteScroll>
```

### ⚠️ Logout and the prefetch cache

`<Link prefetch>` caches pages on the client for 30 seconds. After logout, pressing Back may show a private page from that cache. This happens in the Inertia client and cannot be blocked from the server. Add this to `app.js`:

```js
router.on('navigate', (e) => { if (e.detail.page.clearHistory) router.flushAll() })
```

and call `ginertia.ClearHistory(c)` in your logout handler (see `example/auth.go`).

## Security

The following issues were checked with real attacks and are covered by regression tests in `security_test.go`:

| Risk | How ginertia handles it |
|---|---|
| XSS through props (`</script>`, `<img onerror>`, U+2028, …) | Every `<` in the page JSON is escaped as `\u003c`, whatever `Marshal` you configure. Fuzzed with both an HTML-escaping and a non-escaping encoder, and tested in a real browser with both CSR and SSR. |
| Open redirect via `Referer` in `Back()` | Only same-host referers are followed, reduced to the path; `//evil.com` is rejected. |
| Host header injection (409 version response) | `X-Inertia-Location` uses a relative URL. |
| Protocol-relative redirect via a `//evil.com` request path | Leading slashes are collapsed in the 409 `X-Inertia-Location` and in `page.url`. |
| CSRF | `ginertia.CSRF` middleware: `Sec-Fetch-Site` / `Origin` check plus an HMAC-signed double-submit token that the Inertia client sends automatically. Failures get 419. |
| CSRF from a sibling subdomain (cookie tossing a valid signed token) | Rejected by the origin check (`same-site` requests are not trusted). Reproduced in a real browser by `example/e2e/csrf.py`. Allow extra origins explicitly with `CSRFConfig.TrustedOrigins`. |
| Forged flash cookies | Always HMAC-signed. Without `Secret`, a random key is generated at startup. Secure is enabled automatically over TLS (set `Secure: true` behind a TLS proxy). |
| Replayed / tossed flash cookies | The issue time is signed into the cookie; it is rejected after `CookieFlashStore.MaxAge` (default 5 minutes). |
| Scripts loaded from a stale or planted `public/hot` | `Vite{DisableDev: true}` (production) never reads the hot file. |
| Open redirect via user input (`?next=`) | `Location` is for external URLs and does not restrict the host; check user input with `ginertia.IsLocalURL` first. |
| `Location("javascript:...")` | Only http(s) and relative URLs are allowed. |
| Leaking internal errors | `ValidationErrors` never exposes parser error messages. A failing resolver returns 500 with an empty body. |
| Overwriting other middleware's `Vary` header | Values are appended, not replaced. |

**What you still need to do yourself:**

- **Every prop is sent to the browser.** Passing a GORM `User` struct with a `PasswordHash` field leaks it. Use `json:"-"` or map to a DTO before calling `Render`.
- `Optional` / `Defer` are **not** a way to hide data: any client can request them with a partial reload. Check permissions in the handler.
- Use the same `APP_KEY` on every instance (flash and CSRF), and serve production over HTTPS.
- Behind a reverse proxy, keep the original `Host` header (or list your public origin in `CSRFConfig.TrustedOrigins`), otherwise the CSRF origin check returns 419.
- On logout, call `ginertia.ClearHistory(c)` and add `router.flushAll()` on the client (see the prefetch section above).
- In production set `Vite{DisableDev: true}`, and put a request body size limit in front of `ginertia.CSRF` on routes that do not take large uploads.

## Performance & memory

`_bench/` contains a benchmark server and a load generator that sends mixed traffic: HTML, JSON, partial reloads, deferred props, scroll, failing forms + flash, 409s, prefetch, a dead SSR server and a hanging SSR server. Results on a 2 vCPU VM:

- About 9,500 req/s with p99 around 22 ms. Across 1.7 million requests, post-GC heap stays flat (956 → 970 KB) and goroutine and file descriptor counts do not grow.
- `-race` build under load with concurrent `Share()` calls: 0 data races.

```bash
go build -o benchsrv ./_bench/server && ./benchsrv &
go run ./_bench/load -d 60s -c 64
curl '127.0.0.1:9090/debug/stats?gc=1'
```

## Flash store

By default, flash data and validation errors are stored in the `ginertia_flash` cookie (HttpOnly, SameSite=Lax, HMAC-signed). For larger payloads or server-side storage (Redis, gin-contrib/sessions), implement:

```go
type FlashStore interface {
    Read(c *gin.Context) (*ginertia.FlashData, error)  // read, then delete
    Write(c *gin.Context, d *ginertia.FlashData) error // nil clears it
}
```

## Example app (`example/`)

A demo with login/logout, user CRUD (`useForm`), a login form (`<Form>`), an infinite-scroll feed, polling, prefetch, deferred stats, flash toasts and SSR. Log in with any email and the password `secret`.

```bash
cd example
npm install

# Development: two terminals
npm run dev          # Vite + HMR, writes public/hot
go run .             # http://localhost:8080

# Production: a single binary (views and assets embedded)
npm run build
APP_ENV=production APP_KEY=xxx go build -o app . && APP_ENV=production ./app

# SSR (optional)
npm run ssr &        # node bootstrap/ssr/ssr.js, port 13714
SSR=1 APP_ENV=production ./app
```

To use Vue or React instead of Svelte, nothing changes on the Go side: replace `@inertiajs/svelte` with `@inertiajs/vue3` or `@inertiajs/react` and use the matching Vite plugin.

## Testing

```bash
go test -race ./...
go test -run=^$ -fuzz=FuzzPageScript -fuzztime=60s .
go test -run=^$ -fuzz=FuzzFlashCookie -fuzztime=60s .

# browser security checks (example app running: APP_ENV=production APP_KEY=x ./app)
python3 example/e2e/xss.py CSR
python3 example/e2e/csrf.py
```
