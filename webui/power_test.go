// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 mmBesar

package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
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

// Restart no longer kills a process directly (a captured pid is not reliable
// for a Wine-hosted game -- see the comment on server_marker in start.sh). It
// asks start.sh to stop the server (by writing "stop"), waits for start.sh to
// report it stopped, and then asks it to run again. This test plays the part
// of start.sh: it watches for "stop" and answers by setting state-1=stopped.
func TestPowerRestartWaitsForStopThenAsksToRun(t *testing.T) {
	ts, dir, _ := powerApp(t)

	stopSeen := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			if readFile(t, filepath.Join(dir, "want-1")) == "stop" {
				os.WriteFile(filepath.Join(dir, "state-1"), []byte("stopped\n"), 0o644)
				close(stopSeen)
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()

	if resp, _ := do(t, "POST", ts.URL+"/api/servers/1/power", map[string]string{"action": "restart"}); resp.StatusCode != 200 {
		t.Fatalf("restart: status %d", resp.StatusCode)
	}
	select {
	case <-stopSeen: // good: the handler asked start.sh to stop it
	case <-time.After(2 * time.Second):
		t.Fatal("restart never asked to stop the server")
	}
	if got := readFile(t, filepath.Join(dir, "want-1")); got != "run" {
		t.Errorf("after start.sh reports stopped, restart must ask to run again, got %q", got)
	}
}

// If start.sh never reports the server as stopped, restart must not hang
// forever -- it gives up after its own timeout and asks to run again anyway.
func TestPowerRestartGivesUpIfNeverStopped(t *testing.T) {
	ts, _, _ := powerApp(t)
	start := time.Now()
	if resp, _ := do(t, "POST", ts.URL+"/api/servers/1/power", map[string]string{"action": "restart"}); resp.StatusCode != 200 {
		t.Fatalf("restart: status %d", resp.StatusCode)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Errorf("restart took %s; it should give up well within its own timeout", elapsed)
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
