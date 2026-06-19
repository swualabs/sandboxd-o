package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"sandboxd-o/pkg/auth"
)

type Client struct {
	baseURL      string
	sharedSecret string
	http         *http.Client
}

func New(baseURL string, timeout time.Duration, sharedSecret string) *Client {
	baseURL = strings.TrimSpace(baseURL)
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8082"
	}

	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &Client{baseURL: baseURL, sharedSecret: sharedSecret, http: &http.Client{Timeout: timeout}}
}

func (c *Client) do(ctx context.Context, method, path string, body any) (map[string]any, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}

		rd = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rd)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	auth.SetRequestSecret(req, c.sharedSecret)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode >= 400 {
		var e map[string]any
		_ = json.Unmarshal(raw, &e)
		if m, ok := e["error"].(string); ok && strings.TrimSpace(m) != "" {
			return nil, fmt.Errorf("%s %s: %s", method, path, m)
		}

		if len(raw) == 0 {
			return nil, fmt.Errorf("%s %s: http %d", method, path, res.StatusCode)
		}

		return nil, fmt.Errorf("%s %s: http %d: %s", method, path, res.StatusCode, strings.TrimSpace(string(raw)))
	}

	if len(raw) == 0 {
		return map[string]any{}, nil
	}

	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return out, nil
}
