// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 mmBesar

package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

var tinyJPEG = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 64)...)

func TestLevelTable(t *testing.T) {
	if len(levels) != 61 {
		t.Errorf("expected 61 known levels, got %d", len(levels))
	}
	for id, info := range levels {
		if info.Name == "" || info.Image == "" || id != normalizeLevel(id) {
			t.Errorf("bad entry %q: %+v", id, info)
		}
	}
	if normalizeLevel(" Levels/MP_002 ") != "mp_002" {
		t.Error("normalizeLevel did not lower-case and strip the folder")
	}
}

func TestMapImageDownloadedOnceThenCached(t *testing.T) {
	var hits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.URL.Path != "/Rush/Valapariso_rush_map.jpg" {
			http.NotFound(w, r)
			return
		}
		w.Write(tinyJPEG)
	}))
	defer upstream.Close()

	m := NewMapImages(t.TempDir(), upstream.URL, true)
	for i := 0; i < 3; i++ {
		path, err := m.Path("Levels/MP_002")
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if filepath.Base(path) != "Rush_Valapariso_rush_map.jpg" {
			t.Errorf("unexpected file %s", path)
		}
	}
	if hits != 1 {
		t.Errorf("upstream was asked %d times, want 1", hits)
	}
}

func TestMapImageRefusals(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Conquest/PanamaCanal_cq_map.jpg" {
			w.Write([]byte("<html>not a picture</html>"))
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	m := NewMapImages(t.TempDir(), upstream.URL, true)
	if _, err := m.Path("not_a_map"); err == nil {
		t.Error("unknown map accepted")
	}
	if _, err := m.Path("../../etc/passwd"); err == nil {
		t.Error("path trick accepted")
	}
	if _, err := m.Path("mp_001"); err == nil {
		t.Error("a non-JPEG download was accepted")
	}
	if _, err := m.Path("mp_004"); err == nil {
		t.Error("a 404 from upstream was accepted")
	}

	off := NewMapImages(t.TempDir(), upstream.URL, false)
	if _, err := off.Path("mp_002"); err == nil {
		t.Error("downloads are off but a picture was returned")
	}
}

func TestMapImageRoute(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(tinyJPEG) }))
	defer upstream.Close()

	app := &App{
		byID: map[int]*Server{}, user: "admin", pass: "webpw", started: time.Now(),
		static: fstest.MapFS{"index.html": {Data: []byte("x")}},
		maps:   NewMapImages(t.TempDir(), upstream.URL, true),
	}
	ts := httptest.NewServer(app.routes())
	defer ts.Close()

	get := func(path string, auth bool) *http.Response {
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		if auth {
			req.SetBasicAuth("admin", "webpw")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	if r := get("/maps/mp_002.jpg", false); r.StatusCode != 401 {
		t.Errorf("without login: %d, want 401", r.StatusCode)
	}
	if r := get("/maps/mp_002.jpg", true); r.StatusCode != 200 || r.Header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("picture: %d %s", r.StatusCode, r.Header.Get("Content-Type"))
	}
	if r := get("/maps/nonsense.jpg", true); r.StatusCode != 404 {
		t.Errorf("unknown map: %d, want 404", r.StatusCode)
	}
}

func TestPortListening(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if !portListening(port) {
		t.Errorf("port %d is open but was not seen as listening", port)
	}
	ln.Close()
	if portListening(port) {
		t.Errorf("port %d is closed but was seen as listening", port)
	}
	if portListening(1) {
		t.Error("port 1 reported as listening")
	}
}
