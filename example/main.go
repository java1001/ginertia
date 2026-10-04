// Example app: Gin + ginertia + Svelte 5.
//
//	Dev:   npm run dev   (Vite + HMR)   and   go run .
//	Prod:  npm run build && APP_ENV=production go build -o app . && ./app
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

	// Dev reads views/manifest from disk (edit without recompiling);
	// production serves everything from the binary.
	var files fs.FS = os.DirFS(".")
	if prod {
		files = embedded
		gin.SetMode(gin.ReleaseMode)
	}

	cfg := ginertia.Config{
		RootView:       "views/app.html",
		RootFS:         files,
		ReloadRootView: !prod,
		Vite:           &ginertia.Vite{FS: files, DisableDev: prod},
		Flash:          &ginertia.CookieFlashStore{Secret: []byte(env("APP_KEY", "dev-secret-change-me"))},
	}
	if os.Getenv("SSR") == "1" {
		cfg.SSR = &ginertia.SSR{OnError: func(c *gin.Context, err error) { log.Println("ssr:", err) }}
	}
	inertia := ginertia.MustNew(cfg)
	inertia.Share("appName", "Ginertia Demo")

	r := gin.Default()
	build, _ := fs.Sub(files, "public/build")
	r.StaticFS("/build", http.FS(build))

	sess := &session{secret: []byte(env("APP_KEY", "dev-secret-change-me"))}

	// CSRF for every POST/PUT/PATCH/DELETE. The Inertia client sends the
	// X-XSRF-TOKEN header from the XSRF-TOKEN cookie on its own.
	r.Use(ginertia.CSRF(ginertia.CSRFConfig{Secret: []byte(env("APP_KEY", "dev-secret-change-me"))}))
	r.Use(inertia.Middleware())
	r.Use(sess.shareUser) // auth.user on every page (null for guests)

	h := &handlers{store: newStore(), posts: newPosts(120), sess: sess}

	// Guests
	r.GET("/login", h.loginForm)
	r.POST("/login", h.login)
	r.GET("/docs", func(c *gin.Context) {
		ginertia.Location(c, "https://inertiajs.com")
	})

	// Logged-in users only
	app := r.Group("/", sess.requireAuth)
	app.POST("/logout", h.logout)
	app.GET("/", h.home)
	app.GET("/feed", h.feed)
	app.GET("/users", h.usersIndex)
	app.GET("/users/create", h.usersCreate)
	app.POST("/users", h.usersStore)
	app.GET("/users/:id/edit", h.usersEdit)
	app.PUT("/users/:id", h.usersUpdate)
	app.DELETE("/users/:id", h.usersDestroy)

	addr := env("ADDR", ":8080")
	log.Printf("listening on %s (production=%v)", addr, prod)
	log.Fatal(r.Run(addr))
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
