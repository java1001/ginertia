package ginertia

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"strings"
	"sync"
)

// Vite renders <script>/<link> tags for Vite: the dev server (with HMR) in
// development, the built manifest in production.
//
// Dev mode is on when HotFile exists (the bundled vite plugin writes the dev
// server URL into it while `vite` runs) or when DevURL is set — unless
// DisableDev is true. Set DisableDev in production: otherwise a stale or
// planted hot file in the working directory makes every page load its
// scripts from the URL written in that file.
type Vite struct {
	// DisableDev never uses the dev server: HotFile and DevURL are ignored
	// and the manifest is always used. Recommended: DisableDev: isProduction.
	DisableDev bool

	HotFile      string // default "public/hot"
	DevURL       string // force dev mode, e.g. "http://localhost:5173"
	BaseURL      string // public URL of the build dir, default "/build/"
	ManifestPath string // default "public/build/.vite/manifest.json"
	FS           fs.FS  // read the manifest from here (e.g. embed.FS) instead of disk

	mu       sync.Mutex
	manifest map[string]manifestChunk
}

type manifestChunk struct {
	File    string   `json:"file"`
	Src     string   `json:"src"`
	IsEntry bool     `json:"isEntry"`
	CSS     []string `json:"css"`
	Imports []string `json:"imports"`
}

func (v *Vite) hotFile() string {
	if v.HotFile == "" {
		return "public/hot"
	}
	return v.HotFile
}

func (v *Vite) base() string {
	b := v.BaseURL
	if b == "" {
		b = "/build/"
	}
	if !strings.HasSuffix(b, "/") {
		b += "/"
	}
	return b
}

func (v *Vite) devURL() string {
	if v.DisableDev {
		return ""
	}
	if v.DevURL != "" {
		return strings.TrimRight(v.DevURL, "/")
	}
	if b, err := os.ReadFile(v.hotFile()); err == nil {
		return strings.TrimRight(strings.TrimSpace(string(b)), "/")
	}
	return ""
}

// IsDev reports whether the Vite dev server is in use.
func (v *Vite) IsDev() bool { return v.devURL() != "" }

func (v *Vite) readManifest() ([]byte, error) {
	p := v.ManifestPath
	if p == "" {
		p = "public/build/.vite/manifest.json"
	}
	if v.FS != nil {
		return fs.ReadFile(v.FS, p)
	}
	return os.ReadFile(p)
}

func (v *Vite) loadManifest() (map[string]manifestChunk, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.manifest != nil {
		return v.manifest, nil
	}
	b, err := v.readManifest()
	if err != nil {
		return nil, fmt.Errorf("ginertia: vite manifest not found (run `vite build` or start `vite`): %w", err)
	}
	m := map[string]manifestChunk{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("ginertia: parse vite manifest: %w", err)
	}
	v.manifest = m
	return m, nil
}

// Tags returns the HTML tags for the given entry files (e.g. "src/app.js").
func (v *Vite) Tags(entries ...string) (template.HTML, error) {
	var sb strings.Builder
	esc := template.HTMLEscapeString

	if dev := v.devURL(); dev != "" {
		sb.WriteString(`<script type="module" src="` + esc(dev) + `/@vite/client"></script>`)
		for _, e := range entries {
			if strings.HasSuffix(e, ".css") {
				sb.WriteString(`<link rel="stylesheet" href="` + esc(dev+"/"+e) + `">`)
			} else {
				sb.WriteString(`<script type="module" src="` + esc(dev+"/"+e) + `"></script>`)
			}
		}
		return template.HTML(sb.String()), nil
	}

	m, err := v.loadManifest()
	if err != nil {
		return "", err
	}
	base := v.base()
	seenCSS := map[string]bool{}
	seenImport := map[string]bool{}
	var css, preload, scripts []string

	var walk func(key string)
	walk = func(key string) {
		ch, ok := m[key]
		if !ok {
			return
		}
		for _, c := range ch.CSS {
			if !seenCSS[c] {
				seenCSS[c] = true
				css = append(css, c)
			}
		}
		for _, imp := range ch.Imports {
			if seenImport[imp] {
				continue
			}
			seenImport[imp] = true
			if ic, ok := m[imp]; ok {
				preload = append(preload, ic.File)
			}
			walk(imp)
		}
	}

	for _, e := range entries {
		ch, ok := m[e]
		if !ok {
			return "", fmt.Errorf("ginertia: %q is not in the vite manifest (add it to build.rollupOptions.input)", e)
		}
		if strings.HasSuffix(ch.File, ".css") {
			if !seenCSS[ch.File] {
				seenCSS[ch.File] = true
				css = append(css, ch.File)
			}
			continue
		}
		scripts = append(scripts, ch.File)
		walk(e)
	}

	for _, c := range css {
		sb.WriteString(`<link rel="stylesheet" href="` + esc(base+c) + `">`)
	}
	for _, p := range preload {
		sb.WriteString(`<link rel="modulepreload" href="` + esc(base+p) + `">`)
	}
	for _, s := range scripts {
		sb.WriteString(`<script type="module" src="` + esc(base+s) + `"></script>`)
	}
	return template.HTML(sb.String()), nil
}

// Asset returns the public URL of a source asset processed by Vite
// (e.g. "src/images/logo.png").
func (v *Vite) Asset(path string) (string, error) {
	if dev := v.devURL(); dev != "" {
		return dev + "/" + path, nil
	}
	m, err := v.loadManifest()
	if err != nil {
		return "", err
	}
	ch, ok := m[path]
	if !ok {
		return "", fmt.Errorf("ginertia: %q is not in the vite manifest", path)
	}
	return v.base() + ch.File, nil
}
