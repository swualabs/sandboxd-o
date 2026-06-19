package client

import (
	"net/http"
	"strings"
	"time"
)

type Client struct {
	httpClient   *http.Client
	baseURL      string
	sharedSecret string
}

func New(baseURL string, timeout time.Duration, sharedSecret string) *Client {
	return &Client{
		httpClient:   &http.Client{Timeout: timeout},
		baseURL:      strings.TrimRight(baseURL, "/"),
		sharedSecret: sharedSecret,
	}
}
