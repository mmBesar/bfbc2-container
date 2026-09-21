// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 mmBesar

package main

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// powerApp is an app whose only server points at nothing (its game server is "stopped").
func powerApp(t *testing.T) (*httptest.Server, string, *Server) {
	t.Helper()
	dir := t.TempDir()
	s := &Server{ID: 1, Type: "rush", Rcon: NewClient("127.0.0.1:1", "x"), Control: dir}
	app := &App{
		servers: []*Server{s}, byID: map[int]*Server{1: s},
		user: "admin", pass: "webpw", started: time.Now(), control: dir,
		static: fstest.MapFS{"index.html": {Data: []byte("x")}},
	}
	ts := httptest.NewServer(app.routes())
	t.Cleanup(ts.Close)
	return ts, dir, s
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func TestPowerStartStop(t *testing.T) {
	ts, dir, _ := powerApp(t)

	if resp, _ := do(t, "POST", ts.URL+"/api/servers/1/power", map[string]string{"action": "stop"}); resp.StatusCode != 200 {
		t.Fatalf("stop: status %d", resp.StatusCode)
	}
	if got := readFile(t, filepath.Join(dir, "want-1")); got != "stop" {
		t.Errorf("want-1 = %q, want stop", got)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/servers/1/power", map[string]string{"action": "start"}); resp.StatusCode != 200 {
		t.Fatalf("start: status %d", resp.StatusCode)
	}
	if got := readFile(t, filepath.Join(dir, "want-1")); got != "run" {
		t.Errorf("want-1 = %q, want run", got)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/servers/1/power", map[string]string{"action": "explode"}); resp.StatusCode != 400 {
		t.Errorf("bad action: status %d, want 400", resp.StatusCode)
	}
	if resp, _ := do(t, "POST", ts.URL+"/api/servers/7/power", map[string]string{"action": "stop"}); resp.StatusCode != 404 {
		t.Errorf("unknown server: status %d, want 404", resp.StatusCode)
	}
}

func TestPowerRestartStopsTheProcess(t *testing.T) {
	ts, dir, _ := powerApp(t)
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skip("cannot start a helper process")
	}
	defer cmd.Process.Kill()
	os.WriteFile(filepath.Join(dir, "pid-1"), []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o644)

	if resp, _ := do(t, "POST", ts.URL+"/api/servers/1/power", map[string]string{"action": "restart"}); resp.StatusCode != 200 {
		t.Fatalf("restart: status %d", resp.StatusCode)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done: // the process ended, as start.sh would then notice
	case <-time.After(3 * time.Second):
		t.Error("the game server process was not stopped")
	}
	if got := readFile(t, filepath.Join(dir, "want-1")); got != "run" {
		t.Errorf("after a restart the server must still be wanted running, got %q", got)
	}
}

func TestStoppedServerIsNotAnError(t *testing.T) {
	_, dir, s := powerApp(t)
	os.WriteFile(filepath.Join(dir, "state-1"), []byte("stopped\n"), 0o644)
	s.Poll() // must not try to reach the (non-existent) game server
	v := s.View()
	if v.State != "stopped" || v.Online || v.Error != "" {
		t.Errorf("unexpected view of a stopped server: %+v", v)
	}
}

func TestOwnPicturesWinAndAreListed(t *testing.T) {
	dir := t.TempDir()
	m := NewMapImages(dir, "http://127.0.0.1:1/", false) // downloads off
	if m.Available() {
		t.Error("nothing to show, but Available() is true")
	}
	if _, err := m.Path("mp_002"); err == nil {
		t.Error("a picture was found in an empty folder")
	}
	os.WriteFile(filepath.Join(dir, "mp_002.jpg"), tinyJPEG, 0o644)
	if !m.Available() {
		t.Error("a picture is there, but Available() is false")
	}
	path, err := m.Path("Levels/MP_002")
	if err != nil || filepath.Base(path) != "mp_002.jpg" {
		t.Errorf("own picture not used: %s, %v", path, err)
	}
}
