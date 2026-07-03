package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
)

const ProtocolVersion = "2025-06-18"

type Tool struct {
	Name        string
	Title       string
	Description string
	InputSchema map[string]any
	Handler     func(context.Context, map[string]any) (string, error)
}

type Server struct {
	Name         string
	Title        string
	Version      string
	Instructions string
	Tools        []Tool
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func Serve(ctx context.Context, server Server, stdin io.Reader, stdout io.Writer) error {
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	encoder := json.NewEncoder(stdout)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := scanner.Bytes()
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			if err := encoder.Encode(errorResponse(nil, -32700, "parse error")); err != nil {
				return err
			}
			continue
		}
		if len(req.ID) == 0 {
			if req.Method == "notifications/initialized" {
				continue
			}
			continue
		}
		resp := handle(ctx, server, req)
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func handle(ctx context.Context, server Server, req request) response {
	switch req.Method {
	case "initialize":
		return resultResponse(req.ID, map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			"serverInfo": map[string]any{
				"name":    server.Name,
				"title":   server.Title,
				"version": server.Version,
			},
			"instructions": server.Instructions,
		})
	case "ping":
		return resultResponse(req.ID, map[string]any{})
	case "tools/list":
		tools := make([]map[string]any, 0, len(server.Tools))
		for _, tool := range server.Tools {
			tools = append(tools, map[string]any{
				"name":        tool.Name,
				"title":       tool.Title,
				"description": tool.Description,
				"inputSchema": tool.InputSchema,
			})
		}
		return resultResponse(req.ID, map[string]any{"tools": tools})
	case "tools/call":
		var params toolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return errorResponse(req.ID, -32602, "invalid tools/call params")
		}
		for _, tool := range server.Tools {
			if tool.Name != params.Name {
				continue
			}
			text, err := tool.Handler(ctx, params.Arguments)
			if err != nil {
				return resultResponse(req.ID, toolResult(err.Error(), true))
			}
			return resultResponse(req.ID, toolResult(text, false))
		}
		return errorResponse(req.ID, -32602, fmt.Sprintf("unknown tool: %s", params.Name))
	default:
		return errorResponse(req.ID, -32601, "method not found")
	}
}

func toolResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]string{{
			"type": "text",
			"text": text,
		}},
		"isError": isError,
	}
}

func resultResponse(id json.RawMessage, result any) response {
	return response{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id json.RawMessage, code int, message string) response {
	return response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}
