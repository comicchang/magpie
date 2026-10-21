package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// magpie library mcp add, typed on the command line: each flag is parsed as
// the CLI reads it, and what it saves reaches the agent's own file.
func TestLibraryMCPAddFlagsReachTheAgentFile(t *testing.T) {
	h := t.TempDir()
	for k, v := range map[string]string{"HOME": h, "USERPROFILE": h, "XDG_CONFIG_HOME": filepath.Join(h, ".config"), "CLAUDE_CONFIG_DIR": "", "CODEX_HOME": "", "APPDATA": "", "LOCALAPPDATA": ""} {
		t.Setenv(k, v)
	}
	bin := filepath.Join(h, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// Gemini CLI is found by its binary alone
	if err := os.WriteFile(filepath.Join(bin, "gemini"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	add := func(args ...string) error {
		return libraryCmd(append([]string{"library", "mcp", "add"}, args...))
	}
	// env=K=V splits at its first "=", timeout= is whole seconds, and an
	// argument with "=" that is none of magpie's flags is the command's
	if err := add("fs", "agents=gemini", "env=TOKEN=abc", "env=PLAIN=x=y", "timeout=45", "npx", "-y", "--port=3000", "/tmp"); err != nil {
		t.Fatalf("add over stdio: %v", err)
	}
	if err := add("web", "agents=gemini", "header=X-Api-Key=abc", "https://example.com/mcp"); err != nil {
		t.Fatalf("add over HTTP: %v", err)
	}
	for _, bad := range []string{"timeout=soon", "timeout=-1"} {
		if err := add("bad", "agents=gemini", bad, "npx"); err == nil || !strings.Contains(err.Error(), "timeout") {
			t.Fatalf("%s: got %v, want the timeout refused", bad, err)
		}
	}

	raw, err := os.ReadFile(filepath.Join(h, ".gemini", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("gemini's settings.json is not JSON: %v", err)
	}
	if _, ok := file.MCPServers["bad"]; ok {
		t.Error("a refused add reached the agent's file")
	}
	fs := file.MCPServers["fs"]
	if fs["command"] != "npx" {
		t.Errorf("command = %v, want npx", fs["command"])
	}
	if want := []any{"-y", "--port=3000", "/tmp"}; !reflect.DeepEqual(fs["args"], want) {
		t.Errorf("args = %v, want %v", fs["args"], want)
	}
	if want := map[string]any{"TOKEN": "abc", "PLAIN": "x=y"}; !reflect.DeepEqual(fs["env"], want) {
		t.Errorf("env = %v, want %v", fs["env"], want)
	}
	// gemini-cli reads its timeout in milliseconds
	if fs["timeout"] != float64(45000) {
		t.Errorf("timeout = %v, want 45000 ms", fs["timeout"])
	}
	if want := map[string]any{"X-Api-Key": "abc"}; !reflect.DeepEqual(file.MCPServers["web"]["headers"], want) {
		t.Errorf("web headers = %v, want %v", file.MCPServers["web"]["headers"], want)
	}
}
