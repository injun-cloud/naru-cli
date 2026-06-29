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

	"github.com/injun-cloud/naru-cli/internal/apitypes"
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
	// The stream client has no overall timeout (log follow runs indefinitely) but
	// bounds the response-header wait so a hung server can't block the handshake.
	streamTransport := http.DefaultTransport.(*http.Transport).Clone()
	streamTransport.ResponseHeaderTimeout = 30 * time.Second
	return &Client{
		base:   strings.TrimRight(base, "/"),
		token:  token,
		http:   &http.Client{Timeout: 30 * time.Second, CheckRedirect: noRedirect},
		stream: &http.Client{CheckRedirect: noRedirect, Transport: streamTransport},
	}
}

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
		return s
	}
	// No server-provided hint: add a generic, status-based nudge.
	if e.Status == http.StatusNotFound {
		s += " — check the name with the matching `naru ... ls`"
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
	const maxBody = 8 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if len(data) == maxBody {
		return fmt.Errorf("response exceeded %d-byte limit", maxBody)
	}
	if resp.StatusCode >= 400 {
		return decodeErr(resp.StatusCode, data)
	}
	if out == nil || len(data) == 0 {
		return nil // no body to decode (caller wants nothing, or empty success body)
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
