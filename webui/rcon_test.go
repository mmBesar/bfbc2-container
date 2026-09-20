// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 mmBesar

package main

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
)

func TestPacketRoundTrip(t *testing.T) {
	in := []string{"admin.movePlayer", "Some Name", "1", "0", "true", ""}
	pkt := encodePacket(7, in)
	header, out, err := readPacket(bytes.NewReader(pkt))
	if err != nil {
		t.Fatal(err)
	}
	if header&0x3fffffff != 7 {
		t.Errorf("sequence = %d", header&0x3fffffff)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("got %v, want %v", out, in)
	}
}

func TestReadPacketRejectsGarbage(t *testing.T) {
	// A size field of 4 GB must not make us allocate 4 GB.
	bad := []byte{0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 1, 0, 0, 0}
	if _, _, err := readPacket(bytes.NewReader(bad)); err == nil {
		t.Error("expected an error for an absurd packet size")
	}
	// A word that claims to be longer than the packet.
	short := encodePacket(0, []string{"abc"})
	short[12] = 200
	if _, _, err := readPacket(bytes.NewReader(short)); err == nil {
		t.Error("expected an error for a truncated word")
	}
}

func TestLoginAndCommand(t *testing.T) {
	f := newFake(t, "secret")
	c := NewClient(f.addr(), "secret")
	defer c.Close()
	out, err := c.Do("vars.serverName")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, []string{"Test Rush"}) {
		t.Errorf("got %v", out)
	}
}

func TestWrongPassword(t *testing.T) {
	f := newFake(t, "secret")
	c := NewClient(f.addr(), "wrong")
	defer c.Close()
	if _, err := c.Do("serverInfo"); !errors.Is(err, ErrLogin) {
		t.Errorf("err = %v, want ErrLogin", err)
	}
}

func TestCommandError(t *testing.T) {
	f := newFake(t, "secret")
	c := NewClient(f.addr(), "secret")
	defer c.Close()
	_, err := c.Do("bogus.command")
	var ce *CmdError
	if !errors.As(err, &ce) || ce.Status != "UnknownCommand" {
		t.Errorf("err = %v, want CmdError UnknownCommand", err)
	}
	// The connection must still work after a refused command.
	if _, err := c.Do("vars.serverName"); err != nil {
		t.Errorf("connection broken after a refused command: %v", err)
	}
}

func TestReconnectsAfterDrop(t *testing.T) {
	f := newFake(t, "secret")
	f.dropAt = 1 // on each connection the server answers one command, then hangs up
	c := NewClient(f.addr(), "secret")
	defer c.Close()
	for i := 0; i < 5; i++ {
		if _, err := c.Do("vars.serverName"); err != nil {
			t.Fatalf("command %d failed: %v", i, err)
		}
	}
}

func TestParseServerInfo(t *testing.T) {
	cases := []struct {
		name  string
		words []string
		want  Info
	}{
		{"rush", rushInfo, Info{Name: "Test Rush", Players: 0, MaxPlayers: 24, Mode: "RUSH", Map: "Levels/MP_002",
			RoundsPlayed: 1, RoundsTotal: 2, State: "AcceptingPlayers", Ranked: true, Mod: "BC2", Address: "127.0.0.1:19567", Region: "OC"}},
		{"sqdm", sqdmInfo, Info{Name: "Test SQDM", MaxPlayers: 16, Mode: "SQDM", Map: "Levels/MP_001SDM",
			RoundsPlayed: 1, RoundsTotal: 2, State: "AcceptingPlayers", Ranked: true, Mod: "BC2", Address: "127.0.0.1:19568", Region: "OC"}},
		{"vietnam", vietInfo, Info{Name: "Test Vietnam Rush", MaxPlayers: 24, Mode: "RUSH", Map: "Levels/NAM_MP_002R",
			RoundsPlayed: 1, RoundsTotal: 2, State: "AcceptingPlayers", Ranked: true, Mod: "VIETNAM", Address: "127.0.0.1:19569", Region: "OC"}},
	}
	for _, tc := range cases {
		got, err := parseServerInfo(tc.words)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s:\n got  %+v\n want %+v", tc.name, got, tc.want)
		}
	}
}

func TestParseServerInfoBadInput(t *testing.T) {
	for _, w := range [][]string{nil, {"a"}, {"a", "b", "c", "d", "e", "f", "g", "999", "1", "2", "3", "4"}} {
		if _, err := parseServerInfo(w); err == nil {
			t.Errorf("expected an error for %v", w)
		}
	}
}

func TestParsePlayers(t *testing.T) {
	got, err := parsePlayers([]string{"3", "name", "teamId", "squadId", "2", "Alice", "1", "1", "Bob", "2", "0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0]["name"] != "Alice" || got[1]["teamId"] != "2" {
		t.Errorf("got %v", got)
	}
	if _, err := parsePlayers([]string{"3", "a", "b", "c", "5", "x"}); err == nil {
		t.Error("expected an error for a cut-off list")
	}
	if _, err := parsePlayers([]string{"1"}); err == nil {
		t.Error("expected an error for a too-short list")
	}
}
