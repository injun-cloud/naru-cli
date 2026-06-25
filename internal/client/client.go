// Package client is the naru-server REST client used by both the CLI commands
// and the MCP server.
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/injun-cloud/naru-server/pkg/apitypes"
)

// NotFound reports whether err is a 404 from the server.
func NotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusNotFound
}

// Client talks to naru-server.
type Client struct {
	base   string
	token  string
	http   *http.Client
	stream *http.Client
}

// New builds a client for the given server URL and bearer token.
func New(base, token string) *Client {
	noRedirect := func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse // never follow redirects (avoid token leakage)
	}
	return &Client{
		base:   strings.TrimRight(base, "/"),
		token:  token,
		http:   &http.Client{Timeout: 30 * time.Second, CheckRedirect: noRedirect},
		stream: &http.Client{CheckRedirect: noRedirect}, // no timeout for log streams
	}
}

// Base returns the server base URL.
func (c *Client) Base() string { return c.base }

// Token returns the bearer token.
func (c *Client) Token() string { return c.token }

// APIError is a decoded server error envelope.
type APIError struct {
	Status int
	Code   string
	Msg    string
	Hint   string
}

func (e *APIError) Error() string {
	s := e.Msg
	if e.Hint != "" {
		if e.Code == apitypes.CodeAppNotInstall {
			s += "\n\nInstall the Naru GitHub App at: " + e.Hint
		} else {
			s += " (" + e.Hint + ")"
		}
	}
	return s
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("X-Naru-Client-Schema", apitypes.SchemaVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 400 {
		return decodeErr(resp.StatusCode, data)
	}
	if out == nil {
		return nil
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return json.Unmarshal(env.Data, out)
}

func decodeErr(status int, data []byte) error {
	var env apitypes.ErrorEnvelope
	if json.Unmarshal(data, &env) == nil && env.Error.Code != "" {
		return &APIError{Status: status, Code: env.Error.Code, Msg: env.Error.Message, Hint: env.Error.Hint}
	}
	return &APIError{Status: status, Code: "http_error", Msg: fmt.Sprintf("server returned %d", status)}
}

// Get/Post/Put/Patch/Delete are thin verb wrappers.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}
func (c *Client) Put(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPut, path, body, out)
}
func (c *Client) Patch(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPatch, path, body, out)
}
func (c *Client) Delete(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodDelete, path, nil, out)
}

// Stream opens an SSE stream and calls onLine for each "data:" line.
func (c *Client) Stream(ctx context.Context, path string, onLine func(string)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.stream.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return decodeErr(resp.StatusCode, data)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "data: ") {
			onLine(strings.TrimPrefix(line, "data: "))
		}
	}
	return sc.Err()
}
