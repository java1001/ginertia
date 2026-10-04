package ginertia

import (
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

// A planted/stale hot file must be ignored when DisableDev is set.
func TestViteDisableDevIgnoresHotFile(t *testing.T) {
	dir := t.TempDir()
	hot := filepath.Join(dir, "hot")
	if err := os.WriteFile(hot, []byte("https://evil.example"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := fstest.MapFS{"m.json": {Data: []byte(`{"app.js":{"file":"assets/app-1.js","isEntry":true}}`)}}

	v := &Vite{HotFile: hot, ManifestPath: "m.json", FS: manifest}
	if !v.IsDev() {
		t.Fatal("precondition: hot file should enable dev mode without DisableDev")
	}

	v = &Vite{HotFile: hot, DevURL: "https://evil.example", ManifestPath: "m.json", FS: manifest, DisableDev: true}
	if v.IsDev() {
		t.Fatal("DisableDev: IsDev() must be false")
	}
	tags, err := v.Tags("app.js")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(tags), "evil.example") || !strings.Contains(string(tags), "/build/assets/app-1.js") {
		t.Fatalf("DisableDev: unexpected tags %s", tags)
	}
}

func flashRoundTrip(t *testing.T, store *CookieFlashStore, advance time.Duration) (*FlashData, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	if err := store.Write(c, &FlashData{Flash: map[string]any{"success": "ok"}}); err != nil {
		t.Fatal(err)
	}
	cookie := w.Result().Cookies()[0]
	if cookie.MaxAge <= 0 {
		t.Fatalf("flash cookie should have a MaxAge, got %d", cookie.MaxAge)
	}

	orig := now
	now = func() time.Time { return orig().Add(advance) }
	defer func() { now = orig }()

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c2.Request.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value})
	return store.Read(c2)
}

func TestFlashCookieExpires(t *testing.T) {
	store := &CookieFlashStore{Secret: []byte("k"), MaxAge: time.Minute}

	d, err := flashRoundTrip(t, store, 30*time.Second)
	if err != nil || d == nil || d.Flash["success"] != "ok" {
		t.Fatalf("fresh cookie should be accepted: %v %v", d, err)
	}

	d, err = flashRoundTrip(t, store, 2*time.Minute)
	if err == nil || d != nil {
		t.Fatalf("expired cookie must be rejected, got %v %v", d, err)
	}

	d, err = flashRoundTrip(t, store, -10*time.Minute)
	if err == nil || d != nil {
		t.Fatalf("cookie from the future must be rejected, got %v %v", d, err)
	}
}

// Cookies written by v0.1.x (no issue time) are rejected instead of
// being trusted forever.
func TestFlashCookieWithoutTimestampRejected(t *testing.T) {
	store := &CookieFlashStore{Secret: []byte("k")}
	payload := "eyJmIjp7InN1Y2Nlc3MiOiJvayJ9fQ" // base64url({"f":{"success":"ok"}})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.AddCookie(&http.Cookie{Name: store.name(), Value: payload + "." + store.sign(payload)})
	if d, err := store.Read(c); err == nil || d != nil {
		t.Fatalf("legacy cookie must be rejected, got %v %v", d, err)
	}
}

func TestIsLocalURL(t *testing.T) {
	for in, want := range map[string]bool{
		"/":                    true,
		"/dashboard":           true,
		"/a?b=c#d":             true,
		"/%2F%2Fevil.example":  true, // a path, not a host
		"":                     false,
		"dashboard":            false,
		"//evil.example":       false,
		"/\\evil.example":      false,
		"\\\\evil.example":     false,
		"https://evil.example": false,
		"javascript:alert(1)":  false,
		"/\t/evil.example":     false, // browsers strip tabs → //evil.example
		"/\n/evil.example":     false,
	} {
		if got := IsLocalURL(in); got != want {
			t.Errorf("IsLocalURL(%q) = %v, want %v", in, got, want)
		}
	}
}
