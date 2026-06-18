package tools

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
)

const maxFrameSize = 4 * 1024 * 1024 // 4MB

// SocketRequest is the framed request sent over the Unix domain socket.
type SocketRequest struct {
	Method string         `json:"method"`
	Args   map[string]any `json:"args"`
}

// SocketResponse is the framed response returned over the Unix domain socket.
type SocketResponse struct {
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// RequestHandler processes a single SocketRequest and returns a SocketResponse.
type RequestHandler func(ctx context.Context, req SocketRequest) SocketResponse

// StartSocketServer creates a Unix domain socket at socketPath, removes any
// stale socket file, and accepts connections until ctx is cancelled.
// Each connection is handled in its own goroutine.
func StartSocketServer(ctx context.Context, socketPath string, handler RequestHandler) error {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen unix %s: %w", socketPath, err)
	}
	if err := os.Chmod(socketPath, 0600); err != nil {
		ln.Close()
		return fmt.Errorf("chmod socket %s: %w", socketPath, err)
	}
	slog.Info("uds server listening", "path", socketPath)

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil // clean shutdown
			}
			slog.Error("uds accept error", "err", err)
			continue
		}
		go serveConn(ctx, conn, handler)
	}
}

func serveConn(ctx context.Context, conn net.Conn, handler RequestHandler) {
	defer conn.Close()

	payload, err := ReadFrame(conn)
	if err != nil {
		slog.Debug("uds read frame error", "err", err)
		return
	}

	var req SocketRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		resp := SocketResponse{Error: fmt.Sprintf("unmarshal request: %v", err)}
		writeResponse(conn, resp)
		return
	}

	resp := handler(ctx, req)
	writeResponse(conn, resp)
}

func writeResponse(w io.Writer, resp SocketResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		slog.Error("uds marshal response", "err", err)
		return
	}
	if err := WriteFrame(w, data); err != nil {
		slog.Debug("uds write frame error", "err", err)
	}
}

// ReadFrame reads a length-prefixed frame: 4-byte big-endian uint32 length, then payload.
// Returns an error if the frame exceeds 4MB.
func ReadFrame(r io.Reader) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, fmt.Errorf("read length prefix: %w", err)
	}
	size := binary.BigEndian.Uint32(lenBuf[:])
	if size > maxFrameSize {
		return nil, fmt.Errorf("frame too large: %d bytes (max %d)", size, maxFrameSize)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("read payload: %w", err)
	}
	return payload, nil
}

// WriteFrame writes a length-prefixed frame: 4-byte big-endian uint32 length, then payload.
func WriteFrame(w io.Writer, payload []byte) error {
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(payload)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return fmt.Errorf("write length prefix: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	return nil
}
