package library

import (
	"reflect"
	"testing"
)

// omp's stdio entry: command, args and env, with no type (a command needs
// none, fmtOmp)
func TestOmpStdioEntryCarriesEnv(t *testing.T) {
	omp := &mcpFile{Format: fmtOmp}
	s := &Server{Name: "fs", Transport: "stdio", Command: "npx", Args: []string{"-y", "fs"},
		Env: map[string]string{"TOKEN": "t"}}
	o := omp.encode(s)
	if v, _ := valueOf(o, "command"); v != "npx" {
		t.Errorf("command = %v", v)
	}
	if v, _ := valueOf(o, "args"); !reflect.DeepEqual(v, []string{"-y", "fs"}) {
		t.Errorf("args = %v", v)
	}
	if v, _ := valueOf(o, "env"); !reflect.DeepEqual(v, map[string]string{"TOKEN": "t"}) {
		t.Errorf("env = %v", v)
	}
	if _, ok := valueOf(o, "type"); ok {
		t.Error("a stdio entry wrote a type")
	}
	if _, ok := valueOf(o, "headers"); ok {
		t.Error("a stdio entry wrote headers")
	}
}

// omp's remote entry: type http beside its url, headers and a timeout in
// milliseconds
func TestOmpHTTPEntryCarriesHeadersAndTimeout(t *testing.T) {
	omp := &mcpFile{Format: fmtOmp}
	s := &Server{Name: "api", Transport: "http", URL: "https://x.example/mcp",
		Headers: map[string]string{"Authorization": "Bearer k"}, Timeout: 60}
	o := omp.encode(s)
	if v, _ := valueOf(o, "type"); v != "http" {
		t.Errorf("type = %v, want http", v)
	}
	if v, _ := valueOf(o, "url"); v != "https://x.example/mcp" {
		t.Errorf("url = %v", v)
	}
	if v, _ := valueOf(o, "headers"); !reflect.DeepEqual(v, map[string]string{"Authorization": "Bearer k"}) {
		t.Errorf("headers = %v", v)
	}
	if v, _ := valueOf(o, "timeout"); v != 60000 {
		t.Errorf("timeout = %v, want 60000 ms", v)
	}
	if _, ok := valueOf(o, "command"); ok {
		t.Error("a remote entry wrote a command")
	}
}

// an sse server is written as type sse, not http
func TestOmpSSEEntryKeepsItsType(t *testing.T) {
	omp := &mcpFile{Format: fmtOmp}
	s := &Server{Name: "live", Transport: "sse", URL: "https://x.example/sse"}
	if v, _ := valueOf(omp.encode(s), "type"); v != "sse" {
		t.Errorf("type = %v, want sse", v)
	}
}

// empty env and headers are left out, as omp's own entries leave them
func TestOmpEmptyEnvAndHeadersAreOmitted(t *testing.T) {
	omp := &mcpFile{Format: fmtOmp}
	if _, ok := valueOf(omp.encode(&Server{Name: "fs", Transport: "stdio", Command: "npx"}), "env"); ok {
		t.Error("an empty env was written")
	}
	if _, ok := valueOf(omp.encode(&Server{Name: "api", Transport: "http", URL: "https://x.example/mcp"}), "headers"); ok {
		t.Error("empty headers were written")
	}
}
