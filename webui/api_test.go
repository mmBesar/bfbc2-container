// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 mmBesar

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// testApp starts the app against a fake game server.
func testApp(t *testing.T) (*httptest.Server, *fakeServer) {
	t.Helper()
	f := newFake(t, "rconpw")
	s := &Server{ID: 1, Type: "rush", GamePort: 19567, Rcon: NewClient(f.addr(), "rconpw")}
	s.Poll()
	app := &App{
		servers: []*Server{s}, byID: map[int]*Server{1: s},
		user: "admin", pass: "webpw", started: time.Now(),
		masterConf: filepath.Join(t.TempDir(), "config.ini"),
		static:     fstest.MapFS{"index.html": {Data: []byte("<h1>hi</h1>")}},
	}
	ts := httptest.NewServer(app.routes())
	t.Cleanup(ts.Close)
	return ts, f
}

func do(t *testing.T, method, url string, body any, mods ...func(*http.Request)) (*http.Response, map[string]any) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, url, rd)
	req.SetBasicAuth("admin", "webpw")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, m := range mods {
		m(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func TestAuthRequired(t *testing.T) {
	ts, _ := testApp(t)
	resp, _ := http.Get(ts.URL + "/api/servers")
	if resp.StatusCode != 401 {
		t.Errorf("no credentials: status %d, want 401", resp.StatusCode)
	}
	req, _ := http.NewRequest("GET", ts.URL+"/api/servers", nil)
	req.SetBasicAuth("admin", "wrong")
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != 401 {
		t.Errorf("wrong password: status %d, want 401", resp.StatusCode)
	}
	if resp, _ := http.Get(ts.URL + "/healthz"); resp.StatusCode != 200 {
		t.Errorf("/healthz needs no login, got %d", resp.StatusCode)
	}
	if resp, _ := http.Get(ts.URL + "/"); resp.StatusCode != 401 {
		t.Errorf("the page itself needs a login, got %d", resp.StatusCode)
	}
}

func TestListServers(t *testing.T) {
	ts, _ := testApp(t)
	req, _ := http.NewRequest("GET", ts.URL+"/api/servers", nil)
	req.SetBasicAuth("admin", "webpw")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var views []ServerView
	if err := json.NewDecoder(resp.Body).Decode(&views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || !views[0].Online || views[0].Info == nil || views[0].Info.Name != "Test Rush" {
		t.Errorf("unexpected view: %+v", views)
	}
}

func TestKickAndCsrfProtection(t *testing.T) {
	ts, f := testApp(t)

	resp, out := do(t, "POST", ts.URL+"/api/servers/1/kick", map[string]string{"name": "Bob Smith"})
	if resp.StatusCode != 200 || out["ok"] != true {
		t.Fatalf("kick failed: %d %v", resp.StatusCode, out)
	}
	found := false
	for _, c := range f.recorded() {
		if reflect.DeepEqual(c, []string{"admin.kickPlayer", "Bob Smith"}) {
			found = true // a name with a space arrives as ONE word
		}
	}
	if !found {
		t.Errorf("kick command not received: %v", f.recorded())
	}

	// Not JSON: refused (this is what a form on another website would send).
	req, _ := http.NewRequest("POST", ts.URL+"/api/servers/1/kick", strings.NewReader("name=Bob"))
	req.SetBasicAuth("admin", "webpw")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if r2, _ := http.DefaultClient.Do(req); r2.StatusCode != 415 {
		t.Errorf("form post: status %d, want 415", r2.StatusCode)
	}

	// Right type, but from another site.
	resp, _ = do(t, "POST", ts.URL+"/api/servers/1/kick", map[string]string{"name": "Bob"},
		func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") })
	if resp.StatusCode != 403 {
		t.Errorf("cross-site post: status %d, want 403", resp.StatusCode)
	}
}

func TestBan(t *testing.T) {
	ts, f := testApp(t)
	if resp, _ := do(t, "POST", ts.URL+"/api/servers/1/ban", map[string]any{"name": "Bob", "kind": "seconds", "seconds": 3600}); resp.StatusCode != 200 {
		t.Fatalf("ban failed: %d", resp.StatusCode)
	}
	want := []string{"banList.add", "name", "Bob", "seconds", "3600"}
	ok := false
	for _, c := range f.recorded() {
		ok = ok || reflect.DeepEqual(c, want)
	}
	if !ok {
		t.Errorf("ban command not received: %v", f.recorded())
	}
	for _, bad := range []map[string]any{
		{"name": "Bob", "kind": "forever"},
		{"name": "Bob", "kind": "seconds", "seconds": 0},
		{"name": "", "kind": "perm"},
	} {
		if resp, _ := do(t, "POST", ts.URL+"/api/servers/1/ban", bad); resp.StatusCode != 400 {
			t.Errorf("%v: status %d, want 400", bad, resp.StatusCode)
		}
	}
}

func TestSettings(t *testing.T) {
	ts, f := testApp(t)

	if resp, _ := do(t, "POST", ts.URL+"/api/servers/1/setting", map[string]string{"name": "hardCore", "value": "maybe"}); resp.StatusCode != 400 {
		t.Errorf("bad bool: status %d, want 400", resp.StatusCode)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/servers/1/setting", map[string]string{"name": "evil.command", "value": "x"}); resp.StatusCode != 400 {
		t.Errorf("unknown setting: status %d, want 400", resp.StatusCode)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/servers/1/setting", map[string]string{"name": "gamePassword", "value": "bad pass!"}); resp.StatusCode != 400 {
		t.Errorf("bad password: status %d, want 400", resp.StatusCode)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/servers/1/setting", map[string]string{"name": "serverDescription", "value": "line one\nline two"}); resp.StatusCode != 200 {
		t.Errorf("description: status %d, want 200", resp.StatusCode)
	}
	f.mu.Lock()
	got := f.vars["serverDescription"]
	f.mu.Unlock()
	if got != "line one|line two" {
		t.Errorf("description stored as %q, want line breaks turned into |", got)
	}

	resp, out := do(t, "GET", ts.URL+"/api/servers/1/settings", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("settings: %d", resp.StatusCode)
	}
	values := out["values"].(map[string]any)
	if values["serverName"] != "Test Rush" || out["currentLevel"] != "Levels/MP_002" {
		t.Errorf("unexpected settings: %v", out)
	}
}

func TestRoundAndMove(t *testing.T) {
	ts, f := testApp(t)
	do(t, "POST", ts.URL+"/api/servers/1/round", map[string]any{"action": "end", "team": 2})
	do(t, "POST", ts.URL+"/api/servers/1/move", map[string]any{"name": "Alice", "team": 2, "squad": 3})
	var end, move bool
	for _, c := range f.recorded() {
		end = end || reflect.DeepEqual(c, []string{"admin.endRound", "2"})
		move = move || reflect.DeepEqual(c, []string{"admin.movePlayer", "Alice", "2", "3", "true"})
	}
	if !end || !move {
		t.Errorf("commands not received: %v", f.recorded())
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/servers/1/round", map[string]any{"action": "explode"}); resp.StatusCode != 400 {
		t.Errorf("bad action: status %d", resp.StatusCode)
	}
}

func TestUnknownServer(t *testing.T) {
	ts, _ := testApp(t)
	if resp, _ := do(t, "GET", ts.URL+"/api/servers/9/settings", nil); resp.StatusCode != 404 {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
	if resp, _ := do(t, "GET", ts.URL+"/api/servers/abc/settings", nil); resp.StatusCode != 404 {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
}

func TestSplitCommand(t *testing.T) {
	got, err := splitCommand(`admin.say "hello there" all`)
	if err != nil || !reflect.DeepEqual(got, []string{"admin.say", "hello there", "all"}) {
		t.Errorf("got %v, %v", got, err)
	}
	if got, _ := splitCommand(`vars.gamePassword ""`); !reflect.DeepEqual(got, []string{"vars.gamePassword", ""}) {
		t.Errorf("empty quoted word lost: %v", got)
	}
	if _, err := splitCommand(`admin.say "oops`); err == nil {
		t.Error("expected an error for an unclosed quote")
	}
}

func TestDiscoverServers(t *testing.T) {
	root := t.TempDir()
	write := func(n, content string) {
		os.MkdirAll(filepath.Join(root, n), 0o755)
		os.WriteFile(filepath.Join(root, n, "ServerOptions.ini"), []byte(content), 0o644)
	}
	write("1", "[Options]\nName=One\nPort=19567\nRemoteAdminPort=127.0.0.1:48888\nRemoteAdminPassword=pw1\n")
	write("2", "[Options]\nName=Two\nPort=19568\nRemoteAdminPort=0.0.0.0:48889\nRemoteAdminPassword=pw2\n")
	write("3", "[Options]\nName=Bare\nPort=19569\nRemoteAdminPort=48890\nRemoteAdminPassword=pw3\n")
	env := []string{"SERVER_2_TYPE=SQDM", "SERVER_1_TYPE=rush", "SERVER_3_TYPE=conq", "SERVER_4_TYPE=rush", "SERVER_1_NAME=x"}
	var logs []string
	got := DiscoverServers(env, root, func(f string, a ...any) { logs = append(logs, f) })
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("unexpected servers: %+v (logs %v)", got, logs)
	}
	if got[1].Type != "sqdm" || got[1].GamePort != 19568 || got[1].Rcon.addr != "127.0.0.1:48889" {
		t.Errorf("server 2 = %+v (rcon %s)", got[1], got[1].Rcon.addr)
	}
	if len(logs) != 2 { // server 3 has a bare RCON port, server 4 has no folder
		t.Errorf("expected 2 skipped-server messages, got %v", logs)
	}
}

func TestReadIniValue(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.ini")
	os.WriteFile(p, []byte("[connection]\nplasma_client_port = 18390\t\t; comment\nother = 5\n"), 0o644)
	if v, ok := readIniValue(p, "plasma_client_port"); !ok || v != "18390" {
		t.Errorf("got %q, %v", v, ok)
	}
	if _, ok := readIniValue(p, "missing"); ok {
		t.Error("missing key reported as found")
	}
}
