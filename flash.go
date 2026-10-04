package ginertia

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// FlashData survives exactly one redirect (like Laravel's session flash).
type FlashData struct {
	Errors       map[string]any `json:"e,omitempty"`
	Flash        map[string]any `json:"f,omitempty"`
	ClearHistory bool           `json:"c,omitempty"`
}

func (f *FlashData) empty() bool {
	return f == nil || (len(f.Errors) == 0 && len(f.Flash) == 0 && !f.ClearHistory)
}

// FlashStore persists FlashData between a request and the next one.
// Implement it with your session store (Redis, gin-contrib/sessions, ...)
// if you prefer server-side storage over cookies.
type FlashStore interface {
	// Read returns the stored data and removes it from the store.
	Read(c *gin.Context) (*FlashData, error)
	// Write stores data for the next request; nil clears it.
	Write(c *gin.Context, data *FlashData) error
}

// CookieFlashStore keeps flash data in a short-lived, HMAC-signed cookie.
// Set Secret (same value on every instance). Without it a random key is
// generated at startup: still tamper-proof, but flash data does not survive
// a restart or reach another instance behind a load balancer.
type CookieFlashStore struct {
	Name   string // default "ginertia_flash"
	Secret []byte
	Path   string // default "/"
	Domain string
	Secure bool // auto-enabled on TLS requests

	keyOnce sync.Once
	key     []byte
}

func (s *CookieFlashStore) secret() []byte {
	s.keyOnce.Do(func() {
		if len(s.Secret) > 0 {
			s.key = s.Secret
			return
		}
		s.key = make([]byte, 32)
		_, _ = rand.Read(s.key)
	})
	return s.key
}

var errBadSignature = errors.New("ginertia: invalid flash cookie signature")

func (s *CookieFlashStore) name() string {
	if s.Name == "" {
		return "ginertia_flash"
	}
	return s.Name
}

func (s *CookieFlashStore) Read(c *gin.Context) (*FlashData, error) {
	raw, err := c.Cookie(s.name())
	if err != nil || raw == "" {
		return nil, nil
	}
	s.set(c, "", -1)

	i := strings.LastIndexByte(raw, '.')
	if i < 0 || !hmac.Equal([]byte(raw[i+1:]), []byte(s.sign(raw[:i]))) {
		return nil, errBadSignature
	}
	payload := raw[:i]
	b, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, err
	}
	var d FlashData
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *CookieFlashStore) Write(c *gin.Context, d *FlashData) error {
	if d.empty() {
		s.set(c, "", -1)
		return nil
	}
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	v := base64.RawURLEncoding.EncodeToString(b)
	v += "." + s.sign(v)
	if len(v) > 4000 {
		return errors.New("ginertia: flash data too large for a cookie, use a server-side FlashStore")
	}
	s.set(c, v, 0)
	return nil
}

func (s *CookieFlashStore) sign(v string) string {
	m := hmac.New(sha256.New, s.secret())
	m.Write([]byte(v))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// set writes the cookie, replacing any Set-Cookie for the same name added
// earlier in this response so the last write wins.
func (s *CookieFlashStore) set(c *gin.Context, value string, maxAge int) {
	h := c.Writer.Header()
	prefix := s.name() + "="
	var kept []string
	for _, v := range h.Values("Set-Cookie") {
		if !strings.HasPrefix(v, prefix) {
			kept = append(kept, v)
		}
	}
	h.Del("Set-Cookie")
	for _, v := range kept {
		h.Add("Set-Cookie", v)
	}
	path := s.Path
	if path == "" {
		path = "/"
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     s.name(),
		Value:    value,
		Path:     path,
		Domain:   s.Domain,
		MaxAge:   maxAge,
		Secure:   s.Secure || c.Request.TLS != nil,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ---------------------------------------------------------------------------
// Request helpers
// ---------------------------------------------------------------------------

func (st *state) out() *FlashData {
	if st.outgoing == nil {
		st.outgoing = &FlashData{}
	}
	return st.outgoing
}

func (st *state) persist(c *gin.Context) {
	if err := st.e.cfg.Flash.Write(c, st.outgoing); err != nil {
		_ = c.Error(err)
	}
}

// WithErrors stores validation errors for the next page (after a redirect),
// exposed to the component as the `errors` prop. Honors the error bag sent
// by useForm / router options (X-Inertia-Error-Bag).
func WithErrors[V any](c *gin.Context, errs map[string]V) {
	st := getState(c)
	m := make(map[string]any, len(errs))
	for k, v := range errs {
		m[k] = v
	}
	out := st.out()
	if bag := c.GetHeader(HeaderErrorBag); bag != "" {
		out.Errors = map[string]any{bag: m}
	} else {
		out.Errors = m
	}
	st.persist(c)
}

// Flash stores a value in page.flash of the next page (e.g. a toast).
// On the client: page.flash.success, or router.on("flash", ...).
func Flash(c *gin.Context, key string, value any) {
	st := getState(c)
	out := st.out()
	if out.Flash == nil {
		out.Flash = map[string]any{}
	}
	out.Flash[key] = value
	st.persist(c)
}
