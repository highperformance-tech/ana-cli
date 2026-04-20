package transport

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Option mutates a Client during construction. Options are applied in order by
// New; later options override earlier ones for the same field.
type Option func(*Client)

// WithHTTPClient replaces the HTTP client used for outbound requests. Useful
// for wiring custom transports (recording, retrying, proxying) in tests and
// production alike.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.httpClient = h
		}
	}
}

// WithUserAgent sets the User-Agent header sent on every request. An empty
// string leaves the header unset.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

// Client is a Connect-RPC-over-JSON client. It is safe for concurrent use.
type Client struct {
	httpClient *http.Client
	baseURL    string
	tokenFn    func(context.Context) (string, error)
	userAgent  string
}

// New constructs a Client. baseURL is concatenated with the RPC path on each
// call; tokenFn supplies a bearer token per request (return "" to skip the
// Authorization header). Options may override the default http.Client and set
// a User-Agent.
func New(baseURL string, tokenFn func(context.Context) (string, error), opts ...Option) *Client {
	c := &Client{
		httpClient: http.DefaultClient,
		baseURL:    baseURL,
		tokenFn:    tokenFn,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// joinURL concatenates baseURL and path, collapsing at most one pair of
// adjacent slashes so callers don't need to care whether baseURL has a
// trailing slash or path has a leading one.
func joinURL(baseURL, path string) string {
	if len(baseURL) > 0 && len(path) > 0 && baseURL[len(baseURL)-1] == '/' && path[0] == '/' {
		return baseURL + path[1:]
	}
	return baseURL + path
}

// buildRequest marshals req to JSON and constructs a POST request with all
// standard Connect-over-JSON headers applied. It is shared by Unary and
// Stream so header/body behavior stays in lockstep.
func (c *Client) buildRequest(ctx context.Context, path string, req any) (*http.Request, error) {
	var body []byte
	if req == nil {
		body = []byte("{}")
	} else {
		b, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		body = b
	}

	url := joinURL(c.baseURL, path)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("accept", "application/json")
	if c.userAgent != "" {
		httpReq.Header.Set("User-Agent", c.userAgent)
	}
	if c.tokenFn != nil {
		token, err := c.tokenFn(ctx)
		if err != nil {
			return nil, fmt.Errorf("token: %w", err)
		}
		if token != "" {
			httpReq.Header.Set("Authorization", "Bearer "+token)
		}
	}
	return httpReq, nil
}

// connectErrEnvelope is the flat Connect error body shape.
type connectErrEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// oathkeeperInnerErr is the inner body of the Oathkeeper reverse-proxy
// envelope (the object sitting under the top-level `"error"` key). The
// envelope itself is probed via errEnvelopeShape.Error, so only the nested
// shape needs a named type.
type oathkeeperInnerErr struct {
	Code    int    `json:"code"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// errEnvelopeShape is a probe used to dispatch an error body to the correct
// schema without speculatively unmarshalling both. Connect envelopes carry
// `"code"` as a JSON string at the top level; Oathkeeper envelopes carry
// `"error"` as a nested object. Mutually exclusive in practice, and a probe
// here means an ambiguous body (e.g. one with both keys) takes the Connect
// branch deterministically instead of whichever happens to unmarshal last.
// Field values are kept as json.RawMessage so the decoded slices can be reused
// for the typed decode — no need to re-parse the whole body.
type errEnvelopeShape struct {
	Code    json.RawMessage `json:"code"`
	Message json.RawMessage `json:"message"`
	Error   json.RawMessage `json:"error"`
}

// parseErrorBody turns an HTTP error response into a *Error. It dispatches on
// envelope shape: a top-level JSON-string `"code"` means Connect; a top-level
// JSON-object `"error"` means the Oathkeeper reverse-proxy wrapper. When
// neither shape matches, Raw is populated so the caller still has the bytes.
func parseErrorBody(status int, body []byte) *Error {
	e := &Error{HTTPStatus: status, Raw: body}
	var shape errEnvelopeShape
	if err := json.Unmarshal(body, &shape); err != nil {
		return e
	}
	switch {
	case isJSONString(shape.Code):
		_ = json.Unmarshal(shape.Code, &e.Code)
		_ = json.Unmarshal(shape.Message, &e.Message)
	case isJSONObject(shape.Error):
		var oe oathkeeperInnerErr
		_ = json.Unmarshal(shape.Error, &oe)
		e.Code = oe.Status
		e.Message = oe.Message
	}
	return e
}

// isJSONString reports whether raw is a JSON string value (quote-wrapped).
// A zero-length slice (field absent) returns false.
func isJSONString(raw json.RawMessage) bool {
	return len(raw) > 0 && raw[0] == '"'
}

// isJSONObject reports whether raw is a JSON object value. A zero-length
// slice (field absent) returns false.
func isJSONObject(raw json.RawMessage) bool {
	return len(raw) > 0 && raw[0] == '{'
}

// Unary performs a Connect unary call. req is marshaled as JSON (nil → "{}"),
// resp is populated from the JSON response body (nil skips decoding). On
// non-2xx the response is parsed as a Connect/Oathkeeper error envelope.
func (c *Client) Unary(ctx context.Context, path string, req, resp any) error {
	httpReq, err := c.buildRequest(ctx, path, req)
	if err != nil {
		return err
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		// If the context was cancelled, surface that instead of the
		// lower-level transport error — callers usually care about cancel
		// semantics more than the wrapped net error.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("unary: %w", ctxErr)
		}
		return fmt.Errorf("unary: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return parseErrorBody(httpResp.StatusCode, body)
	}

	if resp == nil {
		return nil
	}
	if err := json.Unmarshal(body, resp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// Stream performs a Connect server-streaming call. On success a *StreamReader
// is returned; the caller must Close it. On non-2xx the response body is
// consumed, parsed as an error envelope, and the returned error is *Error.
func (c *Client) Stream(ctx context.Context, path string, req any) (*StreamReader, error) {
	// Connect server-streaming wraps every message — request and response —
	// in a 5-byte envelope `[flags:1][length:4BE][payload]`. application/json
	// (the unary media type) gets rejected with 415.
	var payload []byte
	if req == nil {
		payload = []byte("{}")
	} else {
		b, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		payload = b
	}
	framed := make([]byte, 5+len(payload))
	binary.BigEndian.PutUint32(framed[1:5], uint32(len(payload)))
	copy(framed[5:], payload)

	url := joinURL(c.baseURL, path)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(framed))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("content-type", "application/connect+json")
	httpReq.Header.Set("accept", "application/connect+json")
	httpReq.Header.Set("connect-protocol-version", "1")
	if c.userAgent != "" {
		httpReq.Header.Set("User-Agent", c.userAgent)
	}
	if c.tokenFn != nil {
		token, err := c.tokenFn(ctx)
		if err != nil {
			return nil, fmt.Errorf("token: %w", err)
		}
		if token != "" {
			httpReq.Header.Set("Authorization", "Bearer "+token)
		}
	}
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("stream: %w", ctxErr)
		}
		return nil, fmt.Errorf("stream: %w", err)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		body, _ := io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		return nil, parseErrorBody(httpResp.StatusCode, body)
	}
	return newStreamReader(httpResp.Body), nil
}
