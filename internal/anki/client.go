// Package anki is a small, typed client for the AnkiConnect add-on.
//
// AnkiConnect speaks a language-agnostic JSON-over-HTTP protocol: every request
// is a POST to the add-on's listening port carrying an action name, a protocol
// version, and an action-specific params object. This package exposes the
// subset of actions Yanki needs and leaves the protocol details here.
//
// See https://foosoft.net/projects/anki-connect/ for the protocol reference.
package anki

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// protocolVersion is the AnkiConnect API version this client targets.
const protocolVersion = 6

// defaultTimeout bounds a single request. Some Anki actions (a full database
// check, an AnkiWeb sync) can legitimately take a while, so this is generous.
const defaultTimeout = 60 * time.Second

// Client talks to a running AnkiConnect instance.
type Client struct {
	baseURL string
	key     string
	http    *http.Client
}

// NewClient returns a client for the AnkiConnect server at baseURL, e.g.
// "http://127.0.0.1:8765". An empty key is allowed for unauthenticated
// instances.
func NewClient(baseURL, key string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		key:     key,
		http:    &http.Client{Timeout: defaultTimeout},
	}
}

// APIError is an error reported by AnkiConnect itself: the request reached
// Anki and Anki rejected it with a message.
type APIError struct {
	Action  string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("anki-connect %s: %s", e.Action, e.Message)
}

// request is the wire format AnkiConnect expects.
type request struct {
	Action  string `json:"action"`
	Version int    `json:"version"`
	Params  any    `json:"params,omitempty"`
	Key     string `json:"key,omitempty"`
}

// response is the wire format AnkiConnect returns. A nil error means success.
type response struct {
	Result json.RawMessage `json:"result"`
	Error  *string         `json:"error"`
}

// invoke performs a single AnkiConnect action, decoding result into out when out
// is non-nil.
func (c *Client) invoke(ctx context.Context, action string, params any, out any) error {
	body, err := json.Marshal(request{
		Action:  action,
		Version: protocolVersion,
		Params:  params,
		Key:     c.key,
	})
	if err != nil {
		return fmt.Errorf("encode %s request: %w", action, err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build %s request: %w", action, err)
	}

	httpRequest.Header.Set("Content-Type", "application/json")

	httpResponse, err := c.http.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("call %s: %w", action, err)
	}
	defer httpResponse.Body.Close()

	payload, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return fmt.Errorf("read %s response: %w", action, err)
	}

	if httpResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("call %s: unexpected HTTP %d: %s", action, httpResponse.StatusCode, strings.TrimSpace(string(payload)))
	}

	var decoded response
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return fmt.Errorf("decode %s response: %w", action, err)
	}

	if decoded.Error != nil {
		return &APIError{Action: action, Message: *decoded.Error}
	}

	if out != nil && len(decoded.Result) > 0 {
		if err := json.Unmarshal(decoded.Result, out); err != nil {
			return fmt.Errorf("decode %s result: %w", action, err)
		}
	}

	return nil
}

// Unreachable reports whether err means AnkiConnect could not be reached at all
// (as opposed to Anki answering with an error).
func Unreachable(err error) bool {
	if err == nil {
		return false
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Connection refused and DNS failures surface as *url.Error wrapping an
	// *net.OpError, so the net.Error check above covers them. Fall back to the
	// error string for other transports.
	message := err.Error()
	return strings.Contains(message, "connection refused") ||
		strings.Contains(message, "no such host") ||
		strings.Contains(message, "connection reset") ||
		strings.Contains(message, "EOF")
}
