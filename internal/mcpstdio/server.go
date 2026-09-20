package mcpstdio

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cottman99/pf-remote/internal/localapi"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

const (
	legacyProtocol = "2025-11-25"
	modernProtocol = "2026-07-28"
	serverName     = "pf-remote"
	serverVersion  = "0.1.0"
	maxMessageSize = 1 << 20
)

type actionClient interface {
	List(context.Context) (contracts.CatalogResponse, error)
	Inspect(context.Context, string) (contracts.InspectResponse, error)
	Context(context.Context, string, string, []string) (contracts.ContextResponse, error)
	Doctor(context.Context) (contracts.DoctorResponse, error)
	Connect(context.Context, string) (contracts.ShellActionResponse, error)
	Exec(context.Context, string, []string) (contracts.ShellActionResponse, error)
	Open(context.Context, string) (contracts.DesktopActionResponse, error)
}

type Server struct {
	Client actionClient
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type callParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

type targetArguments struct {
	Target string `json:"target"`
}

type execArguments struct {
	Target  string   `json:"target"`
	Command []string `json:"command"`
}

func (s Server) Run(ctx context.Context, input io.Reader, output io.Writer) error {
	if s.Client == nil {
		return errors.New("PF Remote MCP requires a local action client")
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), maxMessageSize)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		line := scanner.Bytes()
		var message request
		if err := json.Unmarshal(line, &message); err != nil {
			if err := encoder.Encode(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: -32700, Message: "Parse error"}}); err != nil {
				return err
			}
			continue
		}
		if len(message.ID) == 0 {
			continue
		}
		result, failure := s.handle(ctx, message)
		if err := encoder.Encode(response{JSONRPC: "2.0", ID: message.ID, Result: result, Error: failure}); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s Server) handle(ctx context.Context, message request) (any, *rpcError) {
	if message.JSONRPC != "2.0" {
		return nil, &rpcError{Code: -32600, Message: "Invalid Request"}
	}
	switch message.Method {
	case "server/discover":
		return map[string]any{
			"resultType": "complete", "supportedVersions": []string{modernProtocol},
			"capabilities": map[string]any{"tools": map[string]any{}},
			"instructions": "Use PF Remote targets exactly as supplied. Inspect identity before remote mutation.",
			"ttlMs":        60_000, "cacheScope": "private", "_meta": serverMeta(),
		}, nil
	case "initialize":
		return map[string]any{
			"protocolVersion": legacyProtocol, "capabilities": map[string]any{"tools": map[string]any{}},
			"serverInfo":   map[string]any{"name": serverName, "version": serverVersion},
			"instructions": "Use PF Remote targets exactly as supplied. Inspect identity before remote mutation.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolDefinitions(), "ttlMs": 60_000, "cacheScope": "private", "_meta": serverMeta()}, nil
	case "tools/call":
		return s.callTool(ctx, message.Params)
	default:
		return nil, &rpcError{Code: -32601, Message: "Method not found"}
	}
}

func (s Server) callTool(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var call callParams
	if err := decodeStrict(raw, &call); err != nil || strings.TrimSpace(call.Name) == "" {
		return nil, invalidParams("A tool name and valid arguments are required.")
	}

	var value any
	var err error
	switch call.Name {
	case "pfremote_list":
		if err = requireNoArguments(call.Arguments); err == nil {
			value, err = s.Client.List(ctx)
		}
	case "pfremote_doctor":
		if err = requireNoArguments(call.Arguments); err == nil {
			value, err = s.Client.Doctor(ctx)
		}
	case "pfremote_inspect", "pfremote_context", "pfremote_connect", "pfremote_open":
		var arguments targetArguments
		if err = decodeStrict(call.Arguments, &arguments); err == nil {
			arguments.Target = strings.TrimSpace(arguments.Target)
			if arguments.Target == "" {
				err = errors.New("target is required")
			} else {
				switch call.Name {
				case "pfremote_inspect":
					value, err = s.Client.Inspect(ctx, arguments.Target)
				case "pfremote_context":
					value, err = s.Client.Context(ctx, arguments.Target, "", nil)
				case "pfremote_connect":
					value, err = s.Client.Connect(ctx, arguments.Target)
				case "pfremote_open":
					value, err = s.Client.Open(ctx, arguments.Target)
				}
			}
		}
	case "pfremote_exec":
		var arguments execArguments
		if err = decodeStrict(call.Arguments, &arguments); err == nil {
			arguments.Target = strings.TrimSpace(arguments.Target)
			if arguments.Target == "" || len(arguments.Command) == 0 {
				err = errors.New("target and command are required")
			} else {
				value, err = s.Client.Exec(ctx, arguments.Target, arguments.Command)
			}
		}
	default:
		return nil, invalidParams("Unknown PF Remote tool.")
	}
	if err != nil {
		if errors.Is(err, errInvalidArguments) {
			return nil, invalidParams(err.Error())
		}
		return toolFailure(err), nil
	}
	return toolSuccess(value), nil
}

var errInvalidArguments = errors.New("invalid tool arguments")

func decodeStrict(raw json.RawMessage, destination any) error {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("%w: %v", errInvalidArguments, err)
	}
	return nil
}

func requireNoArguments(raw json.RawMessage) error {
	var arguments struct{}
	return decodeStrict(raw, &arguments)
}

func invalidParams(message string) *rpcError {
	return &rpcError{Code: -32602, Message: "Invalid params", Data: map[string]string{"summary": message}}
}

func toolSuccess(value any) map[string]any {
	encoded, _ := json.Marshal(value)
	return map[string]any{
		"content":           []map[string]any{{"type": "text", "text": string(encoded)}},
		"structuredContent": value, "isError": false, "_meta": serverMeta(),
	}
}

func toolFailure(err error) map[string]any {
	summary := "PF Remote could not complete the requested action."
	remediation := "Inspect the target or run PF Remote diagnostics, then retry."
	var remote *localapi.RemoteError
	if errors.As(err, &remote) {
		summary = remote.Failure.Summary
		remediation = remote.Failure.Remediation
	}
	encoded, _ := json.Marshal(map[string]string{"summary": summary, "remediation": remediation})
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(encoded)}},
		"isError": true, "_meta": serverMeta(),
	}
}

func serverMeta() map[string]any {
	return map[string]any{"io.modelcontextprotocol/serverInfo": map[string]string{"name": serverName, "version": serverVersion}}
}

func toolDefinitions() []map[string]any {
	targetSchema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{"target": map[string]any{"type": "string", "description": "Exact PF Remote canonical target or visible alias."}},
		"required":   []string{"target"},
	}
	noArguments := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}
	return []map[string]any{
		tool("pfremote_list", "List PF Remote targets", "List authorized named computers and capabilities. Use when the user did not supply an exact target.", noArguments, true, false),
		tool("pfremote_inspect", "Inspect a PF Remote target", "Resolve and verify one exact authorized target without connecting to it.", targetSchema, true, false),
		tool("pfremote_context", "Create Agent context", "Return the safe PF_REMOTE_TARGET envelope for one exact authorized target.", targetSchema, true, false),
		tool("pfremote_doctor", "Diagnose PF Remote", "Run redacted local PF Remote health checks.", noArguments, true, false),
		tool("pfremote_connect", "Verify Shell connection", "Establish and verify an authorized Shell session to the exact target.", targetSchema, false, false),
		tool("pfremote_open", "Open remote desktop", "Open the authorized Desktop target using its configured native client.", targetSchema, false, false),
		tool("pfremote_exec", "Execute on remote computer", "Run an exact argument vector on an authorized Shell target. Use only for the user's requested operation.", map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"target":  map[string]any{"type": "string", "description": "Exact PF Remote canonical target or visible alias."},
				"command": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1, "description": "Program and arguments without a shell wrapper."},
			}, "required": []string{"target", "command"},
		}, false, true),
	}
}

func tool(name, title, description string, inputSchema map[string]any, readOnly, destructive bool) map[string]any {
	return map[string]any{
		"name": name, "title": title, "description": description, "inputSchema": inputSchema,
		"annotations": map[string]any{"readOnlyHint": readOnly, "destructiveHint": destructive, "openWorldHint": false},
	}
}
