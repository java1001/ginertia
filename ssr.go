package ginertia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// SSR renders the first page load through the Inertia SSR server
// (`node build/ssr/ssr.js`). On failure the page falls back to client-side
// rendering, so a dead SSR server never takes the site down.
type SSR struct {
	URL     string        // default "http://127.0.0.1:13714"
	Timeout time.Duration // default 2s
	Client  *http.Client
	OnError func(c *gin.Context, err error) // e.g. log it
}

type ssrResult struct {
	Head []string `json:"head"`
	Body string   `json:"body"`
}

func (s *SSR) render(ctx context.Context, pageJSON []byte) (*ssrResult, error) {
	url := s.URL
	if url == "" {
		url = "http://127.0.0.1:13714"
	}
	timeout := s.Timeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(url, "/")+"/render", bytes.NewReader(pageJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ginertia: ssr server returned %d", res.StatusCode)
	}
	var out ssrResult
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Body == "" {
		return nil, fmt.Errorf("ginertia: ssr server returned an empty body")
	}
	return &out, nil
}
