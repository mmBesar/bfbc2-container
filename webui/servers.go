// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 mmBesar

package main

// The game servers this container runs: finding them, and keeping a fresh
// snapshot of each one so that the web page can be answered instantly.

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Server struct {
	ID       int
	Type     string
	GamePort int
	Rcon     *Client

	mu      sync.RWMutex
	info    *Info
	players []map[string]string
	updated time.Time
	lastErr string
	online  bool
}

// ServerView is what the web page receives.
type ServerView struct {
	ID       int                 `json:"id"`
	Type     string              `json:"type"`
	GamePort int                 `json:"gamePort"`
	Online   bool                `json:"online"`
	Error    string              `json:"error,omitempty"`
	Updated  int64               `json:"updated"`
	Info     *Info               `json:"info,omitempty"`
	Players  []map[string]string `json:"players"`
}

func (s *Server) View() ServerView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := ServerView{
		ID: s.ID, Type: s.Type, GamePort: s.GamePort,
		Online: s.online, Error: s.lastErr, Info: s.info,
		Players: s.players,
	}
	if v.Players == nil {
		v.Players = []map[string]string{}
	}
	if !s.updated.IsZero() {
		v.Updated = s.updated.Unix()
	}
	return v
}

// Poll asks the server for its info and player list and stores the result.
func (s *Server) Poll() {
	words, err := s.Rcon.Do("serverInfo")
	if err != nil {
		s.fail(err)
		return
	}
	info, err := parseServerInfo(words)
	if err != nil {
		s.fail(err)
		return
	}
	var players []map[string]string
	if info.Players > 0 {
		pw, err := s.Rcon.Do("admin.listPlayers", "all")
		if err != nil {
			s.fail(err)
			return
		}
		players, err = parsePlayers(pw)
		if err != nil {
			s.fail(err)
			return
		}
	}
	s.mu.Lock()
	s.info, s.players, s.updated, s.online, s.lastErr = &info, players, time.Now(), true, ""
	s.mu.Unlock()
}

func (s *Server) fail(err error) {
	s.mu.Lock()
	s.online, s.lastErr, s.updated = false, err.Error(), time.Now()
	s.mu.Unlock()
}

// StartPolling refreshes the snapshot every few seconds, forever.
func (s *Server) StartPolling(every time.Duration) {
	go func() {
		s.Poll()
		for range time.Tick(every) {
			s.Poll()
		}
	}()
}

// ---- finding the servers -----------------------------------------------------

var typeVar = regexp.MustCompile(`^SERVER_([0-9]+)_TYPE=(.*)$`)

// DiscoverServers reads which servers exist from the environment variables
// (SERVER_<n>_TYPE) and how to reach them from each generated ServerOptions.ini.
func DiscoverServers(environ []string, instanceRoot string, logf func(string, ...any)) []*Server {
	byID := map[int]string{}
	for _, kv := range environ {
		if m := typeVar.FindStringSubmatch(kv); m != nil {
			n, _ := strconv.Atoi(m[1])
			byID[n] = m[2]
		}
	}
	ids := make([]int, 0, len(byID))
	for n := range byID {
		ids = append(ids, n)
	}
	sort.Ints(ids)

	var servers []*Server
	for _, n := range ids {
		file := filepath.Join(instanceRoot, strconv.Itoa(n), "ServerOptions.ini")
		opts, err := readOptions(file)
		if err != nil {
			logf("server %d: skipped, cannot read %s (%v)", n, file, err)
			continue
		}
		host, port, err := net.SplitHostPort(opts["RemoteAdminPort"])
		if err != nil {
			logf("server %d: skipped, RemoteAdminPort %q is not in ip:port form", n, opts["RemoteAdminPort"])
			continue
		}
		if host == "" || host == "0.0.0.0" {
			host = "127.0.0.1"
		}
		servers = append(servers, &Server{
			ID:       n,
			Type:     strings.ToLower(byID[n]),
			GamePort: atoi(opts["Port"]),
			Rcon:     NewClient(net.JoinHostPort(host, port), opts["RemoteAdminPassword"]),
		})
	}
	return servers
}

// readOptions reads the [Options] section of a ServerOptions.ini into a map.
func readOptions(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opts := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "[") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			opts[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return opts, sc.Err()
}

// readIniValue reads one "key = value  ; comment" line from the master's config.ini.
func readIniValue(path, key string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, ";"); i >= 0 {
			line = line[:i]
		}
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v), true
		}
	}
	return "", false
}
