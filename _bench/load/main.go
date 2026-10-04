// Load generator: mixed Inertia traffic, verifies every response status.
package main

import (
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	base    = flag.String("base", "http://127.0.0.1:9090", "")
	dur     = flag.Duration("d", 30*time.Second, "")
	conc    = flag.Int("c", 64, "")
	version string
	client  *http.Client
)

type scenario struct {
	name   string
	weight int
	run    func(prefix string) (int, int) // got, want status
}

func req(method, url string, hdr map[string]string, body string, cookies ...*http.Cookie) *http.Response {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	r, _ := http.NewRequest(method, url, rd)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	res, err := client.Do(r)
	if err != nil {
		return nil
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	return res
}

func status(res *http.Response) int {
	if res == nil {
		return -1
	}
	return res.StatusCode
}

func inertia(extra map[string]string) map[string]string {
	h := map[string]string{"X-Inertia": "true", "X-Inertia-Version": version}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

func main() {
	flag.Parse()
	client = &http.Client{
		Timeout:       10 * time.Second,
		Transport:     &http.Transport{MaxIdleConns: *conc * 2, MaxIdleConnsPerHost: *conc * 2},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	// Discover the asset version from a full page load.
	res, _ := http.Get(*base + "/csr/page")
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	version = regexp.MustCompile(`"version":"([a-f0-9]*)"`).FindStringSubmatch(string(b))[1]

	scenarios := []scenario{
		{"html", 10, func(p string) (int, int) { return status(req("GET", *base+p+"/page", nil, "")), 200 }},
		{"json", 20, func(p string) (int, int) { return status(req("GET", *base+p+"/page", inertia(nil), "")), 200 }},
		{"partial", 10, func(p string) (int, int) {
			return status(req("GET", *base+p+"/page", inertia(map[string]string{"X-Inertia-Partial-Component": "Page", "X-Inertia-Partial-Data": "opt,items"}), "")), 200
		}},
		{"deferred", 8, func(p string) (int, int) {
			return status(req("GET", *base+p+"/page", inertia(map[string]string{"X-Inertia-Partial-Component": "Page", "X-Inertia-Partial-Data": "stats", "X-Inertia-Except-Once-Props": "once"}), "")), 200
		}},
		{"version409", 4, func(p string) (int, int) {
			return status(req("GET", *base+p+"/page", map[string]string{"X-Inertia": "true", "X-Inertia-Version": "old"}, "")), 409
		}},
		{"form-errors+follow", 10, func(p string) (int, int) {
			res := req("POST", *base+p+"/form", inertia(map[string]string{"Content-Type": "application/json", "Referer": *base + p + "/page"}), `{"email":"bad"}`)
			if status(res) != 303 {
				return status(res), 303
			}
			return status(req("GET", *base+p+"/page", inertia(nil), "", res.Cookies()...)), 200
		}},
		{"form-ok", 5, func(p string) (int, int) {
			return status(req("POST", *base+p+"/form", inertia(map[string]string{"Content-Type": "application/json"}), `{"email":"a@b.co"}`)), 303
		}},
		{"put-302to303", 4, func(p string) (int, int) { return status(req("PUT", *base+p+"/form", inertia(nil), "")), 303 }},
		{"scroll", 10, func(p string) (int, int) {
			intent := []string{"append", "prepend"}[rand.IntN(2)]
			return status(req("GET", fmt.Sprintf("%s%s/scroll?page=%d", *base, p, rand.IntN(25)), inertia(map[string]string{"X-Inertia-Partial-Component": "Feed", "X-Inertia-Partial-Data": "posts", "X-Inertia-Infinite-Scroll-Merge-Intent": intent}), "")), 200
		}},
		{"resolver-error", 3, func(p string) (int, int) { return status(req("GET", *base+p+"/fail", inertia(nil), "")), 500 }},
		{"external", 3, func(p string) (int, int) { return status(req("GET", *base+p+"/external", inertia(nil), "")), 409 }},
		{"prefetch", 5, func(p string) (int, int) {
			return status(req("GET", *base+p+"/page", inertia(map[string]string{"Purpose": "prefetch"}), "")), 200
		}},
	}
	var bag []scenario
	for _, s := range scenarios {
		for range s.weight {
			bag = append(bag, s)
		}
	}
	// Engines: CSR, reload-root-view, SSR server down, SSR server hanging.
	prefixes := []string{"/csr", "/csr", "/csr", "/reload", "/ssrdown"}

	var (
		total, bad atomic.Int64
		mu         sync.Mutex
		lat        []time.Duration
		badByName  = map[string]int{}
		wg         sync.WaitGroup
	)
	stop := time.Now().Add(*dur)
	for w := range *conc {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make([]time.Duration, 0, 4096)
			for time.Now().Before(stop) {
				s := bag[rand.IntN(len(bag))]
				p := prefixes[rand.IntN(len(prefixes))]
				if w == 0 && s.name == "html" { // one worker keeps hitting the hanging SSR server
					p = "/ssrhang"
				}
				t := time.Now()
				got, want := s.run(p)
				local = append(local, time.Since(t))
				total.Add(1)
				if got != want {
					bad.Add(1)
					mu.Lock()
					badByName[fmt.Sprintf("%s%s got %d want %d", p, s.name, got, want)]++
					mu.Unlock()
				}
			}
			mu.Lock()
			lat = append(lat, local...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	slices.Sort(lat)
	pct := func(q float64) time.Duration { return lat[int(float64(len(lat)-1)*q)] }
	fmt.Printf("requests=%d rps=%.0f p50=%v p99=%v max=%v unexpected=%d\n",
		total.Load(), float64(total.Load())/dur.Seconds(), pct(.5), pct(.99), lat[len(lat)-1], bad.Load())
	for k, v := range badByName {
		fmt.Printf("  UNEXPECTED %s ×%d\n", k, v)
	}
}
