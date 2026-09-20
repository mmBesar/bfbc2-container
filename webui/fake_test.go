// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 mmBesar

package main

// A fake game server that speaks the RCON protocol, for the tests.

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"net"
	"strings"
	"sync"
	"testing"
)

type fakeServer struct {
	ln       net.Listener
	password string
	dropAt   int // close the connection after this many commands (0 = never)

	mu       sync.Mutex
	vars     map[string]string
	commands [][]string
	info     []string
	players  []string
}

// Real answers copied from a running server (see the CI logs).
var rushInfo = []string{"Test Rush", "0", "24", "RUSH", "Levels/MP_002", "1", "2", "2", "100", "1500", "0",
	"AcceptingPlayers", "true", "false", "false", "5", "5", "BC2", "8", "127.0.0.1:19567", "", "true", "OC"}
var sqdmInfo = []string{"Test SQDM", "0", "16", "SQDM", "Levels/MP_001SDM", "1", "2", "4", "0", "0", "0", "0", "0", "50",
	"AcceptingPlayers", "true", "false", "false", "4", "4", "BC2", "8", "127.0.0.1:19568", "", "true", "OC"}
var vietInfo = []string{"Test Vietnam Rush", "0", "24", "RUSH", "Levels/NAM_MP_002R", "1", "2", "2", "1500", "75", "0",
	"AcceptingPlayers", "true", "false", "false", "2", "2", "VIETNAM", "8", "127.0.0.1:19569", "", "true", "OC"}

func newFake(t *testing.T, password string) *fakeServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeServer{
		ln: ln, password: password,
		vars: map[string]string{"serverName": "Test Rush", "hardCore": "false", "gamePassword": ""},
		info: rushInfo,
		// 3 params, 2 players
		players: []string{"3", "name", "teamId", "squadId", "2", "Alice", "1", "1", "Bob", "2", "0"},
	}
	go f.serve()
	t.Cleanup(func() { ln.Close() })
	return f
}

func (f *fakeServer) addr() string { return f.ln.Addr().String() }

func (f *fakeServer) recorded() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]string, len(f.commands))
	copy(out, f.commands)
	return out
}

func (f *fakeServer) serve() {
	for {
		c, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(c)
	}
}

func (f *fakeServer) handle(c net.Conn) {
	defer c.Close()
	salt := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	count := 0
	for {
		header, words, err := readPacket(c)
		if err != nil || len(words) == 0 {
			return
		}
		if words[0] != "login.hashed" { // logging in does not count as a command
			count++
			if f.dropAt > 0 && count > f.dropAt {
				return // hang up without answering
			}
		}
		seq := header & 0x3fffffff
		reply := f.answer(words, salt)
		buf := encodePacket(seq, reply)
		binary.LittleEndian.PutUint32(buf[0:4], seq|0x80000000|flagResponse)
		c.Write(buf)
	}
}

func (f *fakeServer) answer(words []string, salt []byte) []string {
	if words[0] == "login.hashed" {
		if len(words) == 1 {
			return []string{"OK", strings.ToUpper(hex.EncodeToString(salt))}
		}
		sum := md5.Sum(append(append([]byte{}, salt...), []byte(f.password)...))
		if words[1] == strings.ToUpper(hex.EncodeToString(sum[:])) {
			return []string{"OK"}
		}
		return []string{"InvalidPasswordHash"}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commands = append(f.commands, words)
	switch {
	case words[0] == "serverInfo":
		return append([]string{"OK"}, f.info...)
	case words[0] == "admin.listPlayers":
		return append([]string{"OK"}, f.players...)
	case words[0] == "admin.currentLevel":
		return []string{"OK", "Levels/MP_002"}
	case words[0] == "mapList.list":
		return []string{"OK", "Levels/MP_002", "Levels/MP_004"}
	case strings.HasPrefix(words[0], "vars."):
		name := strings.TrimPrefix(words[0], "vars.")
		if len(words) > 1 {
			f.vars[name] = words[1]
			return []string{"OK"}
		}
		if v, ok := f.vars[name]; ok {
			return []string{"OK", v}
		}
		return []string{"InvalidArguments"}
	case words[0] == "bogus.command":
		return []string{"UnknownCommand"}
	}
	return []string{"OK"}
}
