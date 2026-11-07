package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// frame wraps a JSON-RPC message in the LSP wire format.
func frame(v any) string {
	body, _ := json.Marshal(v)
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
}

// decodeAll parses every framed message in out.
func decodeAll(t *testing.T, out string) []map[string]any {
	t.Helper()
	var msgs []map[string]any
	for {
		i := strings.Index(out, "\r\n\r\n")
		if i < 0 {
			break
		}
		header := out[:i]
		var length int
		fmt.Sscanf(header, "Content-Length: %d", &length)
		start := i + len("\r\n\r\n")
		body := out[start : start+length]
		out = out[start+length:]
		var m map[string]any
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			t.Fatalf("bad message: %v", err)
		}
		msgs = append(msgs, m)
	}
	return msgs
}

func TestInitialize(t *testing.T) {
	in := frame(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{},
	}) + frame(map[string]any{"jsonrpc": "2.0", "method": "exit"})
	var out bytes.Buffer
	if err := Run(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	msgs := decodeAll(t, out.String())
	if len(msgs) == 0 {
		t.Fatal("no response to initialize")
	}
	if msgs[0]["result"] == nil {
		t.Fatalf("initialize had no result: %v", msgs[0])
	}
}

func openDoc(uri, text string) string {
	return frame(map[string]any{
		"jsonrpc": "2.0", "method": "textDocument/didOpen",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": uri, "text": text},
		},
	})
}

func TestDiagnosticsForBadProgram(t *testing.T) {
	docs := openDoc("file:///bad.brick", "function main() {\n    var x: float = 3\n}\n")
	in := docs + frame(map[string]any{"jsonrpc": "2.0", "method": "exit"})
	var out bytes.Buffer
	if err := Run(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	msgs := decodeAll(t, out.String())
	var diags []any
	for _, m := range msgs {
		if m["method"] == "textDocument/publishDiagnostics" {
			diags = m["params"].(map[string]any)["diagnostics"].([]any)
		}
	}
	if len(diags) != 1 {
		t.Fatalf("want 1 diagnostic, got %d: %v", len(diags), diags)
	}
	d := diags[0].(map[string]any)
	if d["severity"].(float64) != 1 {
		t.Errorf("want severity 1 (error)")
	}
	if !strings.Contains(d["message"].(string), "implicit conversion") {
		t.Errorf("unexpected message: %v", d["message"])
	}
}

func TestNoDiagnosticsForGoodProgram(t *testing.T) {
	docs := openDoc("file:///ok.brick", "function main() {\n    print(1)\n}\n")
	in := docs + frame(map[string]any{"jsonrpc": "2.0", "method": "exit"})
	var out bytes.Buffer
	if err := Run(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	for _, m := range decodeAll(t, out.String()) {
		if m["method"] == "textDocument/publishDiagnostics" {
			diags := m["params"].(map[string]any)["diagnostics"].([]any)
			if len(diags) != 0 {
				t.Fatalf("want no diagnostics, got %v", diags)
			}
		}
	}
}

func TestFormatting(t *testing.T) {
	docs := openDoc("file:///f.brick", "function main() {\n      print(1)\n}\n")
	in := docs +
		frame(map[string]any{
			"jsonrpc": "2.0", "id": 2, "method": "textDocument/formatting",
			"params": map[string]any{"textDocument": map[string]any{"uri": "file:///f.brick"}},
		}) +
		frame(map[string]any{"jsonrpc": "2.0", "method": "exit"})
	var out bytes.Buffer
	if err := Run(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, m := range decodeAll(t, out.String()) {
		if m["id"] == float64(2) {
			edits, ok := m["result"].([]any)
			if !ok || len(edits) != 1 {
				t.Fatalf("want one formatting edit, got %v", m["result"])
			}
			newText := edits[0].(map[string]any)["newText"].(string)
			if !strings.Contains(newText, "    print(1)") {
				t.Errorf("formatted text wrong: %q", newText)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no formatting response")
	}
}
