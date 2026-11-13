package push

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

	"github.com/google/uuid"
)

const (
	pushPath            = "/api/v1/push"
	kindRequestPosition = "REQUEST_POSITION"
	maxDrainBytes       = 4 << 10
)

var (
	ErrTransport   = errors.New("push transport error")
	ErrRejected    = errors.New("push rejected the request")
	ErrUnavailable = errors.New("push is unavailable")
)

type request struct {
	RequestID   string            `json:"request_id"`
	RecipientID string            `json:"recipient_id"`
	Kind        string            `json:"kind"`
	Data        map[string]string `json:"data"`
}

type Client struct {
	url  string
	http *http.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: timeout}).DialContext
	transport.TLSHandshakeTimeout = timeout
	transport.ResponseHeaderTimeout = timeout
	return &Client{
		url:  strings.TrimRight(baseURL, "/") + pushPath,
		http: &http.Client{Transport: transport, Timeout: timeout},
	}
}

func (c *Client) Close() {
	c.http.CloseIdleConnections()
}

func (c *Client) RequestPosition(ctx context.Context, tripID, driverID string) error {
	body, err := json.Marshal(request{
		RequestID:   uuid.NewString(),
		RecipientID: driverID,
		Kind:        kindRequestPosition,
		Data:        map[string]string{"trip_id": tripID},
	})
	if err != nil {
		return fmt.Errorf("encode push request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTransport, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBytes))

	switch {
	case resp.StatusCode == http.StatusAccepted:
		return nil
	case resp.StatusCode >= http.StatusBadRequest && resp.StatusCode < http.StatusInternalServerError:
		return fmt.Errorf("%w: status %d", ErrRejected, resp.StatusCode)
	case resp.StatusCode >= http.StatusInternalServerError:
		return fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	default:
		return fmt.Errorf("unexpected push status %d", resp.StatusCode)
	}
}
