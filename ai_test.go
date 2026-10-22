package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGeminiToolRoundTrip checks the generateContent requests against a fake Gemini
// server: the key header, the tool declarations, and that a function call's model turn
// (with its thought signature) is sent back unchanged alongside the function response.
func TestGeminiToolRoundTrip(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "test-key" || !strings.HasSuffix(r.URL.Path, "/test-model:generateContent") {
			http.Error(w, `{"error":{"message":"bad request","status":"INVALID_ARGUMENT"}}`, 400)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		json.Unmarshal(raw, &body)
		bodies = append(bodies, body)
		if len(bodies) == 1 {
			io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call-1","name":"lookup","args":{"q":"x"}},"thoughtSignature":"sig-abc"}]},"finishReason":"STOP"}]}`)
			return
		}
		io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"thinking","thought":true},{"text":"All done."}]},"finishReason":"STOP"}]}`)
	}))
	defer srv.Close()
	old := geminiBase
	geminiBase = srv.URL + "/v1beta/models/"
	defer func() { geminiBase = old }()

	ai := &AI{hc: srv.Client(), apiKey: "test-key", model: "test-model", enabled: true}
	decls := []gFuncDecl{tool("lookup", "Look something up", map[string]any{"q": map[string]any{"type": "string"}}, []string{"q"})}
	contents := []json.RawMessage{userContent(gPart{Text: "hi"})}

	raw, content, err := ai.generate(context.Background(), "system", contents, decls, nil)
	if err != nil {
		t.Fatal(err)
	}
	call := content.Parts[0].FunctionCall
	if call == nil || call.ID != "call-1" || call.Name != "lookup" {
		t.Fatalf("function call not parsed: %+v", content)
	}
	contents = append(contents, raw, userContent(gPart{FunctionResponse: &gFunctionResponse{ID: call.ID, Name: call.Name, Response: map[string]any{"result": 42}}}))
	_, content, err = ai.generate(context.Background(), "system", contents, decls, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := textOf(content); got != "All done." {
		t.Errorf("answer = %q, thought text should be left out", got)
	}

	first, _ := json.Marshal(bodies[0])
	for _, want := range []string{`"systemInstruction":{"parts":[{"text":"system"}]}`, `"functionDeclarations":[{"description":"Look something up","name":"lookup"`} {
		if !strings.Contains(string(first), want) {
			t.Errorf("first request missing %s: %s", want, first)
		}
	}
	second, _ := json.Marshal(bodies[1]["contents"])
	for _, want := range []string{`"thoughtSignature":"sig-abc"`, `"functionResponse":{"id":"call-1","name":"lookup","response":{"result":42}}`} {
		if !strings.Contains(string(second), want) {
			t.Errorf("second request missing %s: %s", want, second)
		}
	}

	// API errors become readable messages.
	ai.apiKey = "wrong"
	if _, err := ai.complete(context.Background(), "s", "p", nil); err == nil || !strings.Contains(err.Error(), "AI request failed") {
		t.Errorf("expected a readable error, got %v", err)
	}
}
