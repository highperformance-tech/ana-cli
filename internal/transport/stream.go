package transport

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// trailerFlag is the bit set on frame flags indicating the frame carries the
// end-of-stream trailer instead of a data payload.
const trailerFlag = 0x02

// StreamReader reads a Connect server-streaming JSON response one frame at a
// time. Each frame is `[flags:1][length:4 BE][payload:length]`. The terminal
// frame has flags&trailerFlag set; its payload is either empty or a JSON
// `{"code","message"}` error envelope.
type StreamReader struct {
	ctx    context.Context
	body   io.ReadCloser
	done   bool // no more data frames coming (trailer seen or error)
	closed bool
}

// newStreamReader wraps an HTTP response body for frame-by-frame reads.
func newStreamReader(ctx context.Context, body io.ReadCloser) *StreamReader {
	return &StreamReader{ctx: ctx, body: body}
}

// Next reads the next data frame and unmarshals its payload into out. It
// returns (true, nil) on a successful data frame, (false, nil) on a clean
// trailer (end of stream), and (false, err) on any error — IO, truncated
// frame, trailer carrying an error envelope, or context cancellation.
func (s *StreamReader) Next(out any) (bool, error) {
	if s.done {
		return false, nil
	}
	if err := s.ctx.Err(); err != nil {
		s.done = true
		return false, fmt.Errorf("stream: %w", err)
	}

	var header [5]byte
	if _, err := io.ReadFull(s.body, header[:]); err != nil {
		s.done = true
		if errors.Is(err, io.EOF) {
			// A bare EOF before any header bytes means the server hung up
			// without a trailer — treat as truncated.
			return false, fmt.Errorf("stream: unexpected EOF before frame header")
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return false, fmt.Errorf("stream: truncated frame header: %w", err)
		}
		if ctxErr := s.ctx.Err(); ctxErr != nil {
			return false, fmt.Errorf("stream: %w", ctxErr)
		}
		return false, fmt.Errorf("stream: read header: %w", err)
	}

	flags := header[0]
	length := binary.BigEndian.Uint32(header[1:5])

	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(s.body, payload); err != nil {
			s.done = true
			if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
				return false, fmt.Errorf("stream: truncated frame payload: %w", err)
			}
			if ctxErr := s.ctx.Err(); ctxErr != nil {
				return false, fmt.Errorf("stream: %w", ctxErr)
			}
			return false, fmt.Errorf("stream: read payload: %w", err)
		}
	}

	if flags&trailerFlag != 0 {
		s.done = true
		if len(payload) == 0 {
			return false, nil
		}
		var env connectErrEnvelope
		if err := json.Unmarshal(payload, &env); err != nil {
			return false, &Error{Raw: payload, Message: string(payload)}
		}
		if env.Code == "" && env.Message == "" {
			return false, nil
		}
		return false, &Error{Code: env.Code, Message: env.Message, Raw: payload}
	}

	if out != nil {
		if err := json.Unmarshal(payload, out); err != nil {
			return false, fmt.Errorf("stream: decode frame: %w", err)
		}
	}
	return true, nil
}

// Close closes the underlying response body. Double-close is safe — the first
// call runs and subsequent calls are no-ops returning nil.
func (s *StreamReader) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
}
