// Package lsp is a small Language Server Protocol implementation for Brick.
//
// It speaks JSON-RPC over stdio with the usual Content-Length framing and
// reuses the pieces the compiler already has: the parser and checker produce
// diagnostics, and the formatter answers a formatting request. It is
// deliberately small -- the goal (spec §11) is editor error underlines and
// formatting, not a full IDE.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"brick/internal/checker"
	"brick/internal/format"
	"brick/internal/parser"
)

// Server handles LSP messages on a single connection.
type Server struct {
	in       *bufio.Reader
	out      io.Writer
	docs     map[string]string
	shutdown bool
}

// Run serves LSP requests from in until the exit notification or EOF.
func Run(in io.Reader, out io.Writer) error {
	s := &Server{
		in:   bufio.NewReader(in),
		out:  out,
		docs: map[string]string{},
	}
	for {
		body, err := s.readMessage()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.handle(body); err != nil {
			return err
		}
		if s.shutdown {
			return nil
		}
	}
}

// ---------- framing ----------

func (s *Server) readMessage() ([]byte, error) {
	length := -1
	for {
		line, err := s.in.ReadString('\n')
		if err != nil {
			if err == io.EOF && line == "" {
				return nil, io.EOF
			}
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			v := strings.TrimSpace(line[len("content-length:"):])
			length, err = strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("bad Content-Length: %v", err)
			}
		}
	}
	if length < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(s.in, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (s *Server) writeMessage(v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n", len(body))
	_, err = s.out.Write(body)
	return err
}

// ---------- dispatch ----------

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (s *Server) handle(body []byte) error {
	var req request
	if err := json.Unmarshal(body, &req); err != nil {
		return nil // ignore malformed input
	}
	switch req.Method {
	case "initialize":
		return s.respond(req.ID, map[string]any{
			"capabilities": map[string]any{
				"textDocumentSync":                1, // full
				"documentFormattingProvider":      true,
				"documentRangeFormattingProvider": false,
			},
			"serverInfo": map[string]any{"name": "brick-lsp", "version": "0.1.0"},
		})
	case "initialized", "$/setTrace":
		return nil
	case "shutdown":
		s.shutdown = true
		return s.respond(req.ID, nil)
	case "exit":
		s.shutdown = true
		return nil
	case "textDocument/didOpen":
		var p struct {
			TextDocument struct {
				URI  string `json:"uri"`
				Text string `json:"text"`
			} `json:"textDocument"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil
		}
		s.docs[p.TextDocument.URI] = p.TextDocument.Text
		return s.publishDiagnostics(p.TextDocument.URI, p.TextDocument.Text)
	case "textDocument/didChange":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil
		}
		if len(p.ContentChanges) > 0 {
			text := p.ContentChanges[len(p.ContentChanges)-1].Text // full sync
			s.docs[p.TextDocument.URI] = text
			return s.publishDiagnostics(p.TextDocument.URI, text)
		}
		return nil
	case "textDocument/didClose":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil
		}
		delete(s.docs, p.TextDocument.URI)
		return s.notify("textDocument/publishDiagnostics", map[string]any{
			"uri": p.TextDocument.URI, "diagnostics": []any{},
		})
	case "textDocument/formatting":
		return s.handleFormatting(req)
	default:
		if req.ID != nil {
			return s.respondError(req.ID, -32601, "method not found: "+req.Method)
		}
		return nil
	}
}

// ---------- responses ----------

func (s *Server) respond(id json.RawMessage, result any) error {
	if id == nil {
		return nil
	}
	return s.writeMessage(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *Server) respondError(id json.RawMessage, code int, msg string) error {
	return s.writeMessage(map[string]any{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": code, "message": msg},
	})
}

func (s *Server) notify(method string, params any) error {
	return s.writeMessage(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// ---------- language features ----------

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type rng struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type diagnostic struct {
	Range    rng    `json:"range"`
	Severity int    `json:"severity"` // 1 = error
	Source   string `json:"source"`
	Message  string `json:"message"`
}

func (s *Server) publishDiagnostics(uri, text string) error {
	return s.notify("textDocument/publishDiagnostics", map[string]any{
		"uri":         uri,
		"diagnostics": diagnostics(text),
	})
}

// diagnostics runs the front end over text and returns any errors as LSP
// diagnostics. Only the first error is reported, since the compiler stops there.
func diagnostics(text string) []diagnostic {
	line, col, msg, ok := firstError(text)
	if !ok {
		return []diagnostic{}
	}
	// Our positions are 1-based; LSP is 0-based.
	p := position{Line: max0(line - 1), Character: max0(col - 1)}
	return []diagnostic{{
		Range:    rng{Start: p, End: p},
		Severity: 1,
		Source:   "brick",
		Message:  msg,
	}}
}

func firstError(text string) (line, col int, msg string, ok bool) {
	prog, err := parser.Parse(text)
	if err != nil {
		if pe, isPE := err.(*parser.ParseError); isPE {
			return pe.Line, pe.Col, pe.Msg, true
		}
		return 1, 1, err.Error(), true
	}
	if _, err := checker.Check(prog); err != nil {
		if ce, isCE := err.(*checker.Error); isCE {
			return ce.Line, ce.Col, ce.Msg, true
		}
		return 1, 1, err.Error(), true
	}
	return 0, 0, "", false
}

func (s *Server) handleFormatting(req request) error {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return s.respondError(req.ID, -32602, "bad params")
	}
	text, ok := s.docs[p.TextDocument.URI]
	if !ok {
		return s.respond(req.ID, []any{})
	}
	formatted := format.Source(text)
	if formatted == text {
		return s.respond(req.ID, []any{})
	}
	edit := map[string]any{
		"range":   rng{Start: position{0, 0}, End: endPosition(text)},
		"newText": formatted,
	}
	return s.respond(req.ID, []any{edit})
}

// endPosition returns the position one past the last character of text.
func endPosition(text string) position {
	lines := strings.Split(text, "\n")
	return position{Line: len(lines) - 1, Character: len(lines[len(lines)-1])}
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
