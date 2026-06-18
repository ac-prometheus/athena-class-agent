package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFrameRoundTrip(t *testing.T) {
	payload := []byte(`{"method":"bash","args":{"command":"ls"}}`)

	var buf bytes.Buffer
	if err := WriteFrame(&buf, payload); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("round-trip mismatch: got %q, want %q", got, payload)
	}
}

func TestFrameRejectsOversized(t *testing.T) {
	var buf bytes.Buffer
	huge := make([]byte, maxFrameSize+1)
	if err := WriteFrame(&buf, huge); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	_, err := ReadFrame(&buf)
	if err == nil {
		t.Error("expected error for oversized frame")
	}
}

func TestSocketServerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	handler := func(_ context.Context, req SocketRequest) SocketResponse {
		if req.Method == "echo" {
			q, _ := req.Args["query"].(string)
			return SocketResponse{Result: "echo: " + q}
		}
		return SocketResponse{Error: "unknown method"}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- StartSocketServer(ctx, sockPath, handler)
	}()

	// Wait for socket to appear.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	req := SocketRequest{Method: "echo", Args: map[string]any{"query": "hello"}}
	payload, _ := json.Marshal(req)
	if err := WriteFrame(conn, payload); err != nil {
		t.Fatalf("write: %v", err)
	}

	respBytes, err := ReadFrame(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	conn.Close()

	var resp SocketResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Result != "echo: hello" {
		t.Errorf("got result %q, want %q", resp.Result, "echo: hello")
	}
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Errorf("server error: %v", err)
	}
}
