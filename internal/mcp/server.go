// Package mcp implements a minimal Model Context Protocol (MCP) server over
// the stdio transport. It exposes orchestrator operations as MCP tools so that
// any MCP-compatible host (Claude desktop, Cursor, etc.) can start and inspect
// agent runs without additional tooling.
//
// Protocol: JSON-RPC 2.0 framed with Content-Length headers, per the MCP
// 2024-11-05 specification.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/nmiano/nickbot/internal/orchestration"
)

const protocolVersion = "2024-11-05"

// Serve runs the MCP stdio server, reading from r and writing to w, until ctx
// is cancelled or r returns io.EOF. It exposes orchestrator operations as MCP
// tools.
func Serve(ctx context.Context, orch *orchestration.Orchestrator, r io.Reader, w io.Writer) error {
	s := &server{
		orch:   orch,
		reader: bufio.NewReader(r),
		writer: w,
	}
	return s.serve(ctx)
}

// ── Internal types ────────────────────────────────────────────────────────────

type server struct {
	orch   *orchestration.Orchestrator
	reader *bufio.Reader
	writer io.Writer
}

type rpcRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Result  any              `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ── Serve loop ────────────────────────────────────────────────────────────────

func (s *server) serve(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg, err := readMessage(s.reader)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("mcp: read: %w", err)
		}

		var req rpcRequest
		if err := json.Unmarshal(msg, &req); err != nil {
			continue // malformed; skip
		}

		// Notifications (no ID) are silently acknowledged.
		if req.ID == nil {
			continue
		}

		result, rpcErr := s.dispatch(ctx, req.Method, req.Params)
		resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
		if rpcErr != nil {
			resp.Error = rpcErr
		} else {
			resp.Result = result
		}
		if err := writeMessage(s.writer, resp); err != nil {
			return fmt.Errorf("mcp: write: %w", err)
		}
	}
}

func (s *server) dispatch(ctx context.Context, method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "initialize":
		return s.handleInitialize(params)
	case "tools/list":
		return s.handleToolsList()
	case "tools/call":
		return s.handleToolsCall(ctx, params)
	case "ping":
		return map[string]any{}, nil
	default:
		return nil, &rpcError{Code: -32601, Message: "method not found: " + method}
	}
}

// ── Method handlers ───────────────────────────────────────────────────────────

type initializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

func (s *server) handleInitialize(_ json.RawMessage) (any, *rpcError) {
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "nickbot",
			"version": "0.2.0",
		},
	}, nil
}

func (s *server) handleToolsList() (any, *rpcError) {
	return map[string]any{
		"tools": toolDefinitions(),
	}, nil
}

type toolsCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func (s *server) handleToolsCall(ctx context.Context, params json.RawMessage) (any, *rpcError) {
	var p toolsCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: -32602, Message: "invalid params: " + err.Error()}
	}

	text, err := callTool(ctx, s.orch, p.Name, p.Arguments)
	if err != nil {
		// MCP tool errors are returned as tool call results with isError=true,
		// not as JSON-RPC errors.
		return toolResult(err.Error(), true), nil
	}
	return toolResult(text, false), nil
}

// toolResult formats a tools/call response.
func toolResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
		"isError": isError,
	}
}

// ── Framing ───────────────────────────────────────────────────────────────────

func readMessage(r *bufio.Reader) ([]byte, error) {
	var contentLength int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break // blank line separates headers from body
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			n, err := strconv.Atoi(strings.TrimPrefix(line, "Content-Length: "))
			if err != nil {
				return nil, fmt.Errorf("bad Content-Length: %w", err)
			}
			contentLength = n
		}
	}
	if contentLength == 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

func writeMessage(w io.Writer, msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "Content-Length: %d\r\n\r\n%s", len(body), body)
	return err
}
