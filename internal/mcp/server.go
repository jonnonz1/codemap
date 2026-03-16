// Package mcp implements a Model Context Protocol (MCP) server over stdio
// using JSON-RPC 2.0. This allows Claude Code to discover and call codemap
// tools natively.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Tool describes an MCP tool that Claude can call.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Handler processes tool calls.
type Handler func(params json.RawMessage) (any, error)

// Server is a stdio MCP server.
type Server struct {
	tools    []Tool
	handlers map[string]Handler
}

// NewServer creates an MCP server.
func NewServer() *Server {
	return &Server{handlers: make(map[string]Handler)}
}

// RegisterTool adds a tool to the server.
func (s *Server) RegisterTool(tool Tool, handler Handler) {
	s.tools = append(s.tools, tool)
	s.handlers[tool.Name] = handler
}

// Run starts the server, reading JSON-RPC requests from stdin and writing
// responses to stdout.
func (s *Server) Run() error {
	reader := bufio.NewReader(os.Stdin)
	writer := os.Stdout

	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			writeError(writer, nil, -32700, "Parse error")
			continue
		}

		s.handleRequest(writer, &req)
	}
}

func (s *Server) handleRequest(w io.Writer, req *jsonRPCRequest) {
	switch req.Method {
	case "initialize":
		writeResult(w, req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]string{
				"name":    "codemap",
				"version": "1.0.0",
			},
		})

	case "notifications/initialized", "initialized":
		// No response needed for notifications.
		return

	case "tools/list":
		writeResult(w, req.ID, map[string]any{
			"tools": s.tools,
		})

	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			writeError(w, req.ID, -32602, "Invalid params")
			return
		}

		handler, ok := s.handlers[params.Name]
		if !ok {
			writeError(w, req.ID, -32602, fmt.Sprintf("Unknown tool: %s", params.Name))
			return
		}

		result, err := handler(params.Arguments)
		if err != nil {
			writeResult(w, req.ID, map[string]any{
				"content": []map[string]string{
					{"type": "text", "text": fmt.Sprintf("Error: %v", err)},
				},
				"isError": true,
			})
			return
		}

		// Marshal result to text.
		var text string
		switch v := result.(type) {
		case string:
			text = v
		default:
			data, _ := json.MarshalIndent(v, "", "  ")
			text = string(data)
		}

		writeResult(w, req.ID, map[string]any{
			"content": []map[string]string{
				{"type": "text", "text": text},
			},
		})

	default:
		writeError(w, req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
	}
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

func writeResult(w io.Writer, id any, result any) {
	resp := jsonRPCResponse{JSONRPC: "2.0", ID: id, Result: result}
	data, _ := json.Marshal(resp)
	fmt.Fprintf(w, "%s\n", data)
}

func writeError(w io.Writer, id any, code int, message string) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   map[string]any{"code": code, "message": message},
	}
	data, _ := json.Marshal(resp)
	fmt.Fprintf(w, "%s\n", data)
}
