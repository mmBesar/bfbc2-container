// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 mmBesar

package main

// Turning the server's list-of-words answers into structured data.
// Everything here is defensive: a short or odd answer gives an error, never a crash.

import (
	"errors"
	"strconv"
)

// Info is what the "serverInfo" command tells us.
type Info struct {
	Name         string `json:"name"`
	Players      int    `json:"players"`
	MaxPlayers   int    `json:"maxPlayers"`
	Mode         string `json:"mode"`
	Map          string `json:"map"`
	RoundsPlayed int    `json:"roundsPlayed"`
	RoundsTotal  int    `json:"roundsTotal"`
	State        string `json:"state"`
	Ranked       bool   `json:"ranked"`
	Punkbuster   bool   `json:"punkbuster"`
	HasPassword  bool   `json:"hasPassword"`
	Mod          string `json:"mod"`
	Address      string `json:"address"`
	Region       string `json:"region"`
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// looksNumeric is true for words like "0", "1500", "-3" or "0.5".
func looksNumeric(s string) bool {
	if s == "" {
		return false
	}
	digits := 0
	for i, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case (r == '-' || r == '+') && i == 0:
		case r == '.':
		default:
			return false
		}
	}
	return digits > 0
}

// parseServerInfo reads the words that follow "OK" in the answer to "serverInfo".
//
// Layout (as seen on real servers):
//
//	name, players, max, mode, map, roundsPlayed, roundsTotal,
//	scoreCount, <scoreCount scores>, <a few more numbers>,
//	state, ranked, punkbuster, hasPassword, ..., mod, ..., address, ..., region
//
// The number of numbers between the scores and the state differs by game mode,
// so we find the state as the first non-numeric word after the scores.
func parseServerInfo(w []string) (Info, error) {
	var info Info
	if len(w) < 12 {
		return info, errors.New("serverInfo answer is too short")
	}
	info.Name = w[0]
	info.Players = atoi(w[1])
	info.MaxPlayers = atoi(w[2])
	info.Mode = w[3]
	info.Map = w[4]
	info.RoundsPlayed = atoi(w[5])
	info.RoundsTotal = atoi(w[6])

	scoreCount := atoi(w[7])
	if scoreCount < 0 || scoreCount > 64 {
		return info, errors.New("serverInfo answer has a strange score count")
	}
	i := 8 + scoreCount
	for i < len(w) && looksNumeric(w[i]) {
		i++
	}
	if i >= len(w) {
		return info, errors.New("serverInfo answer has no state")
	}
	get := func(k int) string {
		if i+k < len(w) {
			return w[i+k]
		}
		return ""
	}
	info.State = w[i]
	info.Ranked = get(1) == "true"
	info.Punkbuster = get(2) == "true"
	info.HasPassword = get(3) == "true"
	info.Mod = get(6)
	info.Address = get(8)
	info.Region = w[len(w)-1]
	return info, nil
}

// parsePlayers reads the answer to "admin.listPlayers all":
//
//	numParams, <names of the params>, numPlayers, <numPlayers * numParams values>
func parsePlayers(w []string) ([]map[string]string, error) {
	if len(w) < 2 {
		return nil, errors.New("player list answer is too short")
	}
	numParams := atoi(w[0])
	if numParams < 0 || numParams > 64 || 1+numParams >= len(w) {
		return nil, errors.New("player list answer has a strange layout")
	}
	names := w[1 : 1+numParams]
	numPlayers := atoi(w[1+numParams])
	rest := w[2+numParams:]
	if numPlayers < 0 || numPlayers*numParams > len(rest) {
		return nil, errors.New("player list answer is cut off")
	}
	players := make([]map[string]string, 0, numPlayers)
	for p := 0; p < numPlayers; p++ {
		row := make(map[string]string, numParams)
		for j, name := range names {
			row[name] = rest[p*numParams+j]
		}
		players = append(players, row)
	}
	return players, nil
}
