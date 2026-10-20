package library

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// a library timeout is seconds; omp's is milliseconds (discovery/mcp-json.ts),
// so it is written ×1000 and read back ÷1000
func TestOmpTimeoutIsMilliseconds(t *testing.T) {
	s := &Server{Name: "fs", Transport: "stdio", Command: "npx", Timeout: 45}
	omp := &mcpFile{Format: fmtOmp}
	if v, ok := valueOf(omp.encode(s), "timeout"); !ok || v != 45000 {
		t.Fatalf("omp timeout = %v %v, want 45000ms", v, ok)
	}
	got, ok := omp.decode("fs", map[string]any{"command": "npx", "timeout": float64(45000)})
	if !ok || got.Timeout != 45 {
		t.Fatalf("read back %+v", got)
	}
}

// no library timeout writes no omp timeout, leaving omp's own 30 seconds
func TestOmpTimeoutLeftOutWhenUnset(t *testing.T) {
	omp := &mcpFile{Format: fmtOmp}
	s := &Server{Name: "fs", Transport: "stdio", Command: "npx"}
	if _, ok := valueOf(omp.encode(s), "timeout"); ok {
		t.Fatal("a timeout was written for a server that set none")
	}
}

// a library timeout is magpie's over the user's old value; an unset one
// leaves the user's timeout exactly as it was
func TestLibraryTimeoutOverridesUserTimeout(t *testing.T) {
	omp := &mcpFile{Format: fmtOmp}
	old := map[string]any{"timeout": float64(30000)}
	set := &Server{Name: "fs", Transport: "stdio", Command: "npx", Timeout: 90}
	if v, _ := valueOf(omp.merged(set, old), "timeout"); v != 90000 {
		t.Errorf("library timeout lost to the user's: %v", v)
	}
	unset := &Server{Name: "fs", Transport: "stdio", Command: "npx"}
	if v, _ := valueOf(omp.merged(unset, old), "timeout"); v != float64(30000) {
		t.Errorf("user's timeout changed without a library one: %v", v)
	}
}

// Codex's tool timeout round-trips, and a library timeout beats the user's
// old tool_timeout_sec
func TestCodexTimeoutRoundTrips(t *testing.T) {
	codex := &mcpFile{Format: fmtCodex}
	s := &Server{Name: "x", Transport: "stdio", Command: "x", Timeout: 120}
	if v, ok := valueOf(codex.encode(s), "tool_timeout_sec"); !ok || v != 120 {
		t.Fatalf("codex tool_timeout_sec = %v %v", v, ok)
	}
	if v, _ := valueOf(codex.merged(s, map[string]any{"tool_timeout_sec": 30}), "tool_timeout_sec"); v != 120 {
		t.Errorf("library timeout lost to the user's 30: %v", v)
	}
	back, _ := codex.decode("x", map[string]any{"command": "x", "tool_timeout_sec": 120})
	if back.Timeout != 120 {
		t.Errorf("read back %d, want 120", back.Timeout)
	}
}

// Goose's 300 is the default magpie gives every server, not one the library
// set, so it reads back as no timeout
func TestGooseDefaultIsNotALibraryTimeout(t *testing.T) {
	goose := &mcpFile{Format: fmtGoose}
	back, _ := goose.decode("x", map[string]any{"type": "stdio", "cmd": "x", "timeout": gooseOldTimeout})
	if back.Timeout != 0 {
		t.Errorf("Goose's default read back as %d", back.Timeout)
	}
}

// the library's comparison: an unset timeout matches whatever the user set,
// so syncing leaves the entry alone; a set one is compared for real
func TestCurrentComparesTimeoutOnlyWhenTheLibrarySetsOne(t *testing.T) {
	omp := &mcpFile{Format: fmtOmp}
	m := map[string]any{"command": "npx", "timeout": float64(30000)}
	unset := &Server{Name: "fs", Transport: "stdio", Command: "npx"}
	if cur, _ := omp.current("fs", m, unset); !cur.same(unset) {
		t.Error("a user timeout made an unset server look out of sync")
	}
	set := &Server{Name: "fs", Transport: "stdio", Command: "npx", Timeout: 90}
	if cur, _ := omp.current("fs", m, set); cur.same(set) {
		t.Error("a library timeout of 90 matched the user's 30")
	}
}

// magpie-image's own tool timeout is magpie's default: it doesn't read back
// as a library timeout, so the self server stays in sync after an upgrade
func TestSelfServerDefaultIsNotALibraryTimeout(t *testing.T) {
	codex := &mcpFile{Format: fmtCodex}
	self := &Server{Name: selfServerName, Transport: "stdio", Command: "/opt/magpie", Args: []string{"mcp", "image"}}
	back, _ := codex.decode(selfServerName, valueMap(codex.encode(self)))
	if back.Timeout != 0 {
		t.Errorf("magpie-image's default read back as a library timeout %d", back.Timeout)
	}
}

// validation: a negative timeout is refused on every save path (check)
func TestNegativeTimeoutIsRefused(t *testing.T) {
	s := Server{Name: "fs", Transport: "stdio", Command: "npx", Timeout: -1}
	if err := s.check(); err == nil {
		t.Fatal("a negative timeout was accepted")
	}
}

// the CLI's timeout reaches the saved entry, end to end
func TestSaveServerKeepsTimeoutInTheAgentFile(t *testing.T) {
	h := sandbox(t)
	p := filepath.Join(h, ".omp", "agent", "mcp.json")
	ok(t)(SaveServer("", Server{Name: "fs", Transport: "stdio", Command: "npx", Timeout: 45, Agents: []string{"omp"}}))
	var file struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(read(t, p)), &file); err != nil {
		t.Fatalf("omp's mcp.json is not JSON: %v", err)
	}
	if got := file.MCPServers["fs"]["timeout"]; got != float64(45000) {
		t.Fatalf("omp's timeout = %v, want 45000 ms", got)
	}
}

// valueMap turns an ordered entry into a plain map, as an agent file reads.
func valueMap(o ordered) map[string]any {
	m := map[string]any{}
	for _, e := range o {
		m[e.k] = e.v
	}
	return m
}
