// Bench server: realistic ginertia routes + /debug/stats for leak checks.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hanh/ginertia"
)

const root = `<!doctype html><html><head>{{ vite "src/app.js" }}{{ .inertiaHead }}</head><body>{{ .inertia }}</body></html>`

type Item struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

func items(n, offset int) []Item {
	out := make([]Item, n)
	for i := range out {
		out[i] = Item{ID: offset + i, Title: fmt.Sprintf("Item %d <b>&", offset+i), Body: "Lorem ipsum dolor sit amet, consectetur adipiscing elit."}
	}
	return out
}

func engine(mod func(*ginertia.Config)) *ginertia.Engine {
	cfg := ginertia.Config{
		RootView: "app.html",
		RootFS:   fstest.MapFS{"app.html": {Data: []byte(root)}, "m.json": {Data: []byte(`{"src/app.js":{"file":"assets/app.js","isEntry":true,"css":["assets/app.css"]}}`)}},
		Vite:     &ginertia.Vite{HotFile: "/nonexistent", FS: fstest.MapFS{"m.json": {Data: []byte(`{"src/app.js":{"file":"assets/app.js","isEntry":true,"css":["assets/app.css"]}}`)}}, ManifestPath: "m.json"},
		Flash:    &ginertia.CookieFlashStore{Secret: []byte("bench")},
	}
	if mod != nil {
		mod(&cfg)
	}
	e := ginertia.MustNew(cfg)
	e.Share("app", "bench")
	e.Share("now", func() any { return time.Now().Unix() })
	return e
}

func routes(g *gin.RouterGroup, e *ginertia.Engine) {
	g.Use(e.Middleware())
	g.Use(func(c *gin.Context) {
		ginertia.Share(c, "auth", gin.H{"user": gin.H{"id": 1, "name": "bench"}})
		c.Next()
	})
	g.GET("/page", func(c *gin.Context) {
		ginertia.Render(c, "Page", gin.H{
			"items":  items(50, 0),
			"opt":    ginertia.Optional(func() any { return items(20, 100) }),
			"stats":  ginertia.Defer(func() any { return gin.H{"total": 50} }),
			"merged": ginertia.Merge(items(5, 200)).MatchOn("id"),
			"once":   ginertia.Once(func() any { return "plans" }).Until(time.Hour),
		})
	})
	g.GET("/scroll", func(c *gin.Context) {
		p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		ginertia.Render(c, "Feed", gin.H{"posts": ginertia.ScrollPage(items(15, p*15), p, p < 20)})
	})
	g.POST("/form", func(c *gin.Context) {
		var in struct {
			Email string `json:"email" binding:"required,email"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			ginertia.WithErrors(c, ginertia.ValidationErrors(err, in))
			ginertia.Flash(c, "error", "fix it")
			ginertia.Back(c)
			return
		}
		ginertia.Flash(c, "success", "ok")
		ginertia.Redirect(c, c.Request.URL.Path[:len(c.Request.URL.Path)-len("/form")]+"/page")
	})
	g.PUT("/form", func(c *gin.Context) { c.Redirect(http.StatusFound, "/page") })
	g.GET("/fail", func(c *gin.Context) {
		ginertia.Render(c, "X", gin.H{"x": func() (any, error) { return nil, fmt.Errorf("db down") }})
	})
	g.GET("/external", func(c *gin.Context) { ginertia.Location(c, "https://example.com") })
}

func main() {
	gin.SetMode(gin.ReleaseMode)

	// A "black hole" SSR server: accepts TCP connections but never answers,
	// so every SSR call hits the timeout — the worst case for goroutine leaks.
	hole, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() {
		for {
			c, err := hole.Accept()
			if err != nil {
				return
			}
			// Never answer; close only when the client gives up.
			go func() { io.Copy(io.Discard, c); c.Close() }()
		}
	}()

	r := gin.New()
	r.Use(gin.Recovery())
	csr := engine(nil)
	routes(r.Group("/csr"), csr)
	// Mutate shared props concurrently with rendering (race detector bait).
	go func() {
		for i := 0; ; i++ {
			csr.Share("tick", i)
			time.Sleep(time.Millisecond)
		}
	}()
	routes(r.Group("/reload"), engine(func(c *ginertia.Config) { c.ReloadRootView = true }))
	routes(r.Group("/ssrdown"), engine(func(c *ginertia.Config) { c.SSR = &ginertia.SSR{URL: "http://127.0.0.1:1"} }))
	routes(r.Group("/ssrhang"), engine(func(c *ginertia.Config) {
		c.SSR = &ginertia.SSR{URL: "http://" + hole.Addr().String(), Timeout: 150 * time.Millisecond}
	}))

	r.GET("/debug/stats", func(c *gin.Context) {
		if c.Query("gc") == "1" {
			runtime.GC()
			debug.FreeOSMemory()
		}
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		json.NewEncoder(c.Writer).Encode(gin.H{
			"heapAllocKB": m.HeapAlloc / 1024, "heapInuseKB": m.HeapInuse / 1024, "sysKB": m.Sys / 1024,
			"heapObjects": m.HeapObjects, "goroutines": runtime.NumGoroutine(), "numGC": m.NumGC,
		})
	})
	r.GET("/debug/pprof/*any", gin.WrapH(http.DefaultServeMux))

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "127.0.0.1:9090"
	}
	log.Fatal(r.Run(addr))
}
