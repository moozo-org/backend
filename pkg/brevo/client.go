// Package brevo is a minimal client for Brevo's API
// (https://developers.brevo.com). It knows nothing about the application, so
// other channels Brevo offers, such as SMS, can be added alongside email.
package brevo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultBaseURL is Brevo's v3 API.
const DefaultBaseURL = "https://api.brevo.com/v3"

const defaultTimeout = 10 * time.Second

// Client calls Brevo's API with one API key.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

type Option func(*Client)

// WithBaseURL sends requests to url instead of DefaultBaseURL, such as a test
// server.
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

// WithHTTPClient replaces the default client, which times out after 10s.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.http = hc }
}

func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:  apiKey,
		baseURL: DefaultBaseURL,
		http:    &http.Client{Timeout: defaultTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// APIError is a non-2xx answer from Brevo. Code and Message come from its
// error body, when it has one.
type APIError struct {
	StatusCode int
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("brevo: %d %s", e.StatusCode, http.StatusText(e.StatusCode))
	}
	return fmt.Sprintf("brevo: %d %s: %s", e.StatusCode, e.Code, e.Message)
}

// post sends body as JSON to path and returns an *APIError for a non-2xx
// answer.
func (c *Client) post(ctx context.Context, path string, body any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("brevo: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode >= 200 && res.StatusCode <= 299 {
		return nil
	}
	apiErr := &APIError{StatusCode: res.StatusCode}
	// Best effort: the status alone is still a usable error.
	_ = json.NewDecoder(io.LimitReader(res.Body, 1<<10)).Decode(apiErr)
	return apiErr
}
