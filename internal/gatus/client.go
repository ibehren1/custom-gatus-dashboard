package gatus

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Client talks to the Gatus HTTP API.
type Client struct {
	baseURL string
	http    *http.Client

	mu    sync.Mutex
	title string
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Statuses returns all endpoint statuses with up to resultCount recent results each.
func (c *Client) Statuses(ctx context.Context, resultCount int) ([]EndpointStatus, error) {
	url := fmt.Sprintf("%s/api/v1/endpoints/statuses?page=1&pageSize=%d", c.baseURL, resultCount)
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	var statuses []EndpointStatus
	if err := json.Unmarshal(body, &statuses); err != nil {
		return nil, fmt.Errorf("decode gatus statuses: %w", err)
	}
	return statuses, nil
}

var titleTag = regexp.MustCompile(`(?is)<title>(.*?)</title>`)

// Title returns the page title configured in Gatus (ui.title), or "" if unavailable.
func (c *Client) Title(ctx context.Context) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.title != "" {
		return c.title
	}
	body, err := c.get(ctx, c.baseURL+"/")
	if err != nil {
		return ""
	}
	if m := titleTag.FindSubmatch(body); m != nil {
		c.title = strings.TrimSpace(html.UnescapeString(string(m[1])))
	}
	return c.title
}

func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request %s: unexpected status %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}
