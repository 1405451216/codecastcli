package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// LSP types
// ---------------------------------------------------------------------------

// Position represents a position in a text document (0-based line and character).
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range represents a span in a text document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location represents a location inside a resource.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// Diagnostic represents a diagnostic item.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Message  string `json:"message"`
}

// ---------------------------------------------------------------------------
// JSON-RPC types
// ---------------------------------------------------------------------------

type jsonrpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type jsonrpcNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// ---------------------------------------------------------------------------
// LSP initialize types
// ---------------------------------------------------------------------------

type initializeParams struct {
	ProcessID     int                `json:"processId"`
	RootURI       string             `json:"rootUri"`
	Capabilities  clientCapabilities `json:"capabilities"`
	WorkspaceFolders []workspaceFolder `json:"workspaceFolders,omitempty"`
}

type clientCapabilities struct {
	TextDocument textDocumentClientCapabilities `json:"textDocument"`
}

type textDocumentClientCapabilities struct {
	Definition      *definitionCapabilities      `json:"definition,omitempty"`
	References      *referencesCapabilities      `json:"references,omitempty"`
	Hover           *hoverCapabilities           `json:"hover,omitempty"`
	PublishDiagnostics *publishDiagnosticsCapabilities `json:"publishDiagnostics,omitempty"`
}

type definitionCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration"`
	LinkSupport        bool `json:"linkSupport"`
}

type referencesCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration"`
}

type hoverCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration"`
	ContentFormat      []string `json:"contentFormat,omitempty"`
}

type publishDiagnosticsCapabilities struct {
	RelatedInformation bool `json:"relatedInformation"`
}

type workspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

type textDocumentPositionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type referenceParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      referenceContext       `json:"context"`
}

type referenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

type initializeResult struct {
	Capabilities interface{} `json:"capabilities"`
}

type hoverResult struct {
	Contents markupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

type markupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type diagnosticParams struct {
	URI string `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

// Client implements an LSP JSON-RPC client that communicates with a running
// language server process over stdio.
type Client struct {
	server       *Server
	nextID       atomic.Int64
	pending      map[int64]chan json.RawMessage
	mu           sync.Mutex
	initialized  bool
	ctx          context.Context
	cancel       context.CancelFunc
	diagnostics  map[string][]Diagnostic
	diagMu       sync.RWMutex
}

// newClient creates a new LSP Client for the given server.
func newClient(server *Server) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		server:      server,
		pending:     make(map[int64]chan json.RawMessage),
		ctx:         ctx,
		cancel:      cancel,
		diagnostics: make(map[string][]Diagnostic),
	}
}

// Start begins reading responses from the server and performs the LSP
// initialize handshake.
func (c *Client) Start(rootURI string) error {
	go c.readResponses()

	if err := c.Initialize(rootURI); err != nil {
		c.cancel()
		return fmt.Errorf("LSP initialize 握手失败: %w", err)
	}

	c.mu.Lock()
	c.initialized = true
	c.mu.Unlock()
	return nil
}

// Stop cancels the client context and cleans up pending requests.
func (c *Client) Stop() {
	c.cancel()
	c.mu.Lock()
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.mu.Unlock()
}

// ---------------------------------------------------------------------------
// JSON-RPC transport
// ---------------------------------------------------------------------------

// sendRequest sends a JSON-RPC request and waits for the response with a 10s
// timeout. It returns the raw result payload.
func (c *Client) sendRequest(method string, params interface{}) (json.RawMessage, error) {
	id := c.nextID.Add(1)

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	ch := make(chan json.RawMessage, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.writeMessage(req); err != nil {
		return nil, fmt.Errorf("发送请求 %s 失败: %w", method, err)
	}

	select {
	case result, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("LSP 客户端已关闭 (请求: %s)", method)
		}
		return result, nil
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("LSP 请求 %s 超时 (10s)", method)
	case <-c.ctx.Done():
		return nil, fmt.Errorf("LSP 客户端已取消 (请求: %s)", method)
	}
}

// sendNotification sends a JSON-RPC notification (no response expected).
func (c *Client) sendNotification(method string, params interface{}) error {
	var paramsRaw json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("序列化通知参数失败: %w", err)
		}
		paramsRaw = data
	}
	notif := jsonrpcNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsRaw,
	}
	return c.writeMessage(notif)
}

// readResponses is a long-running goroutine that reads LSP responses from the
// server's stdout and dispatches them to the appropriate pending channel.
func (c *Client) readResponses() {
	reader := bufio.NewReader(c.server.stdout)
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		// Read Content-Length header
		var contentLength int
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if c.ctx.Err() != nil {
					return
				}
				// Server likely exited
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			if strings.HasPrefix(line, "Content-Length:") {
				clStr := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
				if n, err := strconv.Atoi(clStr); err == nil {
					contentLength = n
				}
			}
		}

		if contentLength <= 0 {
			continue
		}

		// Read the JSON body
		buf := make([]byte, contentLength)
		if _, err := reader.Read(buf); err != nil {
			if c.ctx.Err() != nil {
				return
			}
			return
		}

		// Try to parse as response (has "id")
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(buf, &raw); err != nil {
			continue
		}

		if idRaw, hasID := raw["id"]; hasID {
			var id int64
			if err := json.Unmarshal(idRaw, &id); err != nil {
				continue
			}

			// Check if it's an error response
			if errRaw, hasErr := raw["error"]; hasErr {
				var rpcErr jsonrpcError
				_ = json.Unmarshal(errRaw, &rpcErr)

				c.mu.Lock()
				ch, ok := c.pending[id]
				c.mu.Unlock()

				if ok {
					// Send a nil result; the error is logged but the caller
					// gets an empty response. We wrap the error info in a
					// synthetic result so sendRequest can detect it.
					errResult, _ := json.Marshal(map[string]interface{}{
						"__lsp_error": true,
						"code":        rpcErr.Code,
						"message":     rpcErr.Message,
					})
					select {
					case ch <- errResult:
					default:
					}
				}
				continue
			}

			// Normal response
			resultRaw, _ := raw["result"]

			c.mu.Lock()
			ch, ok := c.pending[id]
			c.mu.Unlock()

			if ok {
				select {
				case ch <- resultRaw:
				default:
				}
			}
		} else if methodRaw, hasMethod := raw["method"]; hasMethod {
			// Notification from server
			var method string
			_ = json.Unmarshal(methodRaw, &method)
			if method == "textDocument/publishDiagnostics" {
				var notif jsonrpcNotification
				if err := json.Unmarshal(buf, &notif); err == nil {
					var diag diagnosticParams
					if err := json.Unmarshal(notif.Params, &diag); err == nil {
						c.diagMu.Lock()
						c.diagnostics[diag.URI] = diag.Diagnostics
						c.diagMu.Unlock()
					}
				}
			}
		}
	}
}

// writeMessage writes a JSON-RPC message to the server's stdin with the
// Content-Length header.
func (c *Client) writeMessage(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化 JSON-RPC 消息失败: %w", err)
	}

	c.server.mu.Lock()
	defer c.server.mu.Unlock()

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := c.server.stdin.Write([]byte(header)); err != nil {
		return fmt.Errorf("写入消息头失败: %w", err)
	}
	if _, err := c.server.stdin.Write(data); err != nil {
		return fmt.Errorf("写入消息体失败: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// LSP protocol methods
// ---------------------------------------------------------------------------

// Initialize sends the LSP initialize request followed by the initialized
// notification to complete the handshake.
func (c *Client) Initialize(rootURI string) error {
	params := initializeParams{
		ProcessID: 0,
		RootURI:   rootURI,
		Capabilities: clientCapabilities{
			TextDocument: textDocumentClientCapabilities{
				Definition: &definitionCapabilities{
					LinkSupport: true,
				},
				References: &referencesCapabilities{},
				Hover: &hoverCapabilities{
					ContentFormat: []string{"markdown", "plaintext"},
				},
				PublishDiagnostics: &publishDiagnosticsCapabilities{
					RelatedInformation: true,
				},
			},
		},
		WorkspaceFolders: []workspaceFolder{
			{URI: rootURI, Name: "root"},
		},
	}

	result, err := c.sendRequest("initialize", params)
	if err != nil {
		return err
	}

	// Check for LSP error in result
	var errCheck map[string]interface{}
	if json.Unmarshal(result, &errCheck) == nil {
		if isErr, _ := errCheck["__lsp_error"].(bool); isErr {
			return fmt.Errorf("LSP initialize 错误: %v", errCheck["message"])
		}
	}

	// Verify we got a valid result
	var initResult initializeResult
	if err := json.Unmarshal(result, &initResult); err != nil {
		return fmt.Errorf("解析 initialize 结果失败: %w", err)
	}

	// Send initialized notification
	if err := c.sendNotification("initialized", map[string]interface{}{}); err != nil {
		return fmt.Errorf("发送 initialized 通知失败: %w", err)
	}

	return nil
}

// Shutdown sends the LSP shutdown request.
func (c *Client) Shutdown() error {
	_, err := c.sendRequest("shutdown", nil)
	return err
}

// GotoDefinition sends textDocument/definition and returns the locations.
func (c *Client) GotoDefinition(uri string, line, character int) ([]Location, error) {
	params := textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: character},
	}

	result, err := c.sendRequest("textDocument/definition", params)
	if err != nil {
		return nil, err
	}

	return parseLocations(result)
}

// FindReferences sends textDocument/references and returns the locations.
func (c *Client) FindReferences(uri string, line, character int) ([]Location, error) {
	params := referenceParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: character},
		Context:      referenceContext{IncludeDeclaration: true},
	}

	result, err := c.sendRequest("textDocument/references", params)
	if err != nil {
		return nil, err
	}

	return parseLocations(result)
}

// Hover sends textDocument/hover and returns the hover text.
func (c *Client) Hover(uri string, line, character int) (string, error) {
	params := textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: character},
	}

	result, err := c.sendRequest("textDocument/hover", params)
	if err != nil {
		return "", err
	}

	if result == nil {
		return "", nil
	}

	// Check for LSP error
	var errCheck map[string]interface{}
	if json.Unmarshal(result, &errCheck) == nil {
		if isErr, _ := errCheck["__lsp_error"].(bool); isErr {
			return "", fmt.Errorf("LSP hover 错误: %v", errCheck["message"])
		}
	}

	var hover hoverResult
	if err := json.Unmarshal(result, &hover); err != nil {
		// Try as string (some servers return plain string)
		var s string
		if json.Unmarshal(result, &s) == nil {
			return s, nil
		}
		return "", nil
	}

	return hover.Contents.Value, nil
}

// Diagnostics returns the cached diagnostics for the given URI.
// LSP servers push diagnostics via textDocument/publishDiagnostics
// notifications, so this returns whatever has been received.
func (c *Client) Diagnostics(uri string) ([]Diagnostic, error) {
	c.diagMu.RLock()
	defer c.diagMu.RUnlock()
	diags := c.diagnostics[uri]
	if diags == nil {
		return []Diagnostic{}, nil
	}
	// Return a copy
	result := make([]Diagnostic, len(diags))
	copy(result, diags)
	return result, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseLocations handles the different response shapes that
// textDocument/definition and textDocument/references can return:
//   - null / empty
//   - single Location
//   - []Location
//   - LocationLink[] (simplified: extract URI + target range)
func parseLocations(raw json.RawMessage) ([]Location, error) {
	if raw == nil || string(raw) == "null" {
		return []Location{}, nil
	}

	// Check for LSP error
	var errCheck map[string]interface{}
	if json.Unmarshal(raw, &errCheck) == nil {
		if isErr, _ := errCheck["__lsp_error"].(bool); isErr {
			return nil, fmt.Errorf("LSP 错误: %v", errCheck["message"])
		}
	}

	// Try as array
	var locs []Location
	if err := json.Unmarshal(raw, &locs); err == nil {
		return locs, nil
	}

	// Try as single Location
	var loc Location
	if err := json.Unmarshal(raw, &loc); err == nil {
		if loc.URI != "" {
			return []Location{loc}, nil
		}
	}

	// Try as LocationLink[]
	type locationLink struct {
		TargetURI            string `json:"targetUri"`
		TargetRange          Range  `json:"targetRange"`
		TargetSelectionRange Range  `json:"targetSelectionRange"`
	}
	var links []locationLink
	if err := json.Unmarshal(raw, &links); err == nil {
		result := make([]Location, len(links))
		for i, l := range links {
			result[i] = Location{
				URI:   l.TargetURI,
				Range: l.TargetSelectionRange,
			}
		}
		return result, nil
	}

	// Try as single LocationLink
	var link locationLink
	if err := json.Unmarshal(raw, &link); err == nil {
		if link.TargetURI != "" {
			return []Location{{
				URI:   link.TargetURI,
				Range: link.TargetSelectionRange,
			}}, nil
		}
	}

	return []Location{}, nil
}

// FileURI converts a file path to a file:// URI.
func FileURI(path string) string {
	u := url.URL{Scheme: "file", Path: path}
	return u.String()
}
