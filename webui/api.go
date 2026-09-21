// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 mmBesar

package main

// The JSON API behind the web page.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const version = "0.3.0"

type App struct {
	servers    []*Server
	byID       map[int]*Server
	user, pass string
	masterConf string // path of the master's config.ini (to find its ports)
	maps       *MapImages
	control    string // folder for starting and stopping servers (see start.sh)
	started    time.Time
	static     fs.FS
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	mux.HandleFunc("GET /api/status", a.handleStatus)
	mux.HandleFunc("GET /api/servers", a.handleServers)
	mux.HandleFunc("GET /api/servers/{id}/settings", a.handleSettings)
	mux.HandleFunc("POST /api/servers/{id}/power", a.handlePower)
	mux.HandleFunc("POST /api/servers/{id}/kick", a.handleKick)
	mux.HandleFunc("POST /api/servers/{id}/ban", a.handleBan)
	mux.HandleFunc("POST /api/servers/{id}/move", a.handleMove)
	mux.HandleFunc("POST /api/servers/{id}/round", a.handleRound)
	mux.HandleFunc("POST /api/servers/{id}/setting", a.handleSetting)
	mux.HandleFunc("POST /api/servers/{id}/console", a.handleConsole)
	mux.HandleFunc("GET /maps/{file}", a.handleMapImage)
	mux.Handle("/", http.FileServer(http.FS(a.static)))
	return a.secure(mux)
}

// ---- small helpers -----------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return false
	}
	return true
}

// server finds the server named in the URL, or answers 404.
func (a *App) server(w http.ResponseWriter, r *http.Request) *Server {
	id, err := strconv.Atoi(r.PathValue("id"))
	if s := a.byID[id]; err == nil && s != nil {
		return s
	}
	writeErr(w, http.StatusNotFound, "no such server")
	return nil
}

// word checks text that is sent to the game server as one word.
func word(s string, max int) (string, error) {
	if len(s) > max {
		return "", fmt.Errorf("too long (max %d characters)", max)
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return "", errors.New("contains a control character")
		}
	}
	return s, nil
}

// run sends a command and answers the web request with the result.
func (a *App) run(w http.ResponseWriter, s *Server, words ...string) ([]string, bool) {
	out, err := s.Rcon.Do(words...)
	if err != nil {
		var ce *CmdError
		if errors.As(err, &ce) {
			writeErr(w, http.StatusBadRequest, ce.Error())
		} else {
			writeErr(w, http.StatusBadGateway, err.Error())
		}
		return nil, false
	}
	go s.Poll() // refresh the snapshot so the page shows the change soon
	return out, true
}

func okJSON(w http.ResponseWriter, out []string) {
	if out == nil {
		out = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": out})
}

// handleMapImage sends the picture of a map, like /maps/mp_002.jpg
func (a *App) handleMapImage(w http.ResponseWriter, r *http.Request) {
	if a.maps == nil {
		http.NotFound(w, r)
		return
	}
	file, err := a.maps.Path(strings.TrimSuffix(r.PathValue("file"), ".jpg"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, file)
}

// ---- read-only endpoints ----------------------------------------------------------

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	type port struct {
		Name string `json:"name"`
		Port int    `json:"port"`
		Open bool   `json:"open"`
	}
	var ports []port
	all := true
	for _, p := range []struct{ key, name string }{
		{"plasma_client_port", "clients (plasma)"},
		{"theater_client_port", "clients (theater)"},
		{"plasma_server_port", "servers (plasma)"},
		{"theater_server_port", "servers (theater)"},
	} {
		v, found := readIniValue(a.masterConf, p.key)
		n := atoi(v)
		if !found || n == 0 {
			continue
		}
		open := portListening(n) // reads the socket tables: no connection, so nothing in the master's log
		all = all && open
		ports = append(ports, port{p.name, n, open})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":   version,
		"uptime":    int(time.Since(a.started).Seconds()),
		"servers":   len(a.servers),
		"mapImages": a.maps != nil && a.maps.Available(),
		"master":    map[string]any{"ports": ports, "reachable": len(ports) > 0 && all},
	})
}

func (a *App) handleServers(w http.ResponseWriter, r *http.Request) {
	views := make([]ServerView, 0, len(a.servers))
	for _, s := range a.servers {
		views = append(views, s.View())
	}
	writeJSON(w, http.StatusOK, views)
}

// The settings the page can show and change. "kind" tells the page what input to draw.
type settingDef struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // bool, int, text or password
	Max  int    `json:"max"`
}

var settingDefs = []settingDef{
	{"serverName", "text", 100},
	{"serverDescription", "text", 400},
	{"gamePassword", "password", 16},
	{"bannerUrl", "text", 200},
	{"hardCore", "bool", 0},
	{"friendlyFire", "bool", 0},
	{"teamBalance", "bool", 0},
	{"killCam", "bool", 0},
	{"miniMap", "bool", 0},
	{"crossHair", "bool", 0},
	{"3dSpotting", "bool", 0},
	{"miniMapSpotting", "bool", 0},
	{"thirdPersonVehicleCameras", "bool", 0},
	{"profanityFilter", "bool", 0},
	{"idleTimeout", "int", 0},
	{"teamKillCountForKick", "int", 0},
	{"teamKillValueForKick", "int", 0},
	{"teamKillValueIncrease", "int", 0},
	{"teamKillValueDecreasePerSecond", "int", 0},
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	s := a.server(w, r)
	if s == nil {
		return
	}
	values := map[string]string{}
	for _, d := range settingDefs {
		out, err := s.Rcon.Do("vars." + d.Name)
		if err != nil {
			var ce *CmdError
			if !errors.As(err, &ce) { // the server is not reachable at all
				writeErr(w, http.StatusBadGateway, err.Error())
				return
			}
			continue // this one setting is not available: leave it out
		}
		if len(out) > 0 {
			values[d.Name] = out[0]
		}
	}
	level, _ := s.Rcon.Do("admin.currentLevel")
	maps, _ := s.Rcon.Do("mapList.list")
	current := ""
	if len(level) > 0 {
		current = level[0]
	}
	if maps == nil {
		maps = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"defs": settingDefs, "values": values, "currentLevel": current, "maps": maps,
	})
}

// handlePower starts, stops or restarts a game server. It does not touch the
// game itself: it leaves a note for start.sh (the "want" file), which then does it.
// Starting and stopping last until the container restarts. After that, the
// SERVER_<n>_AUTOSTART setting decides again.
func (a *App) handlePower(w http.ResponseWriter, r *http.Request) {
	s := a.server(w, r)
	if s == nil {
		return
	}
	if a.control == "" {
		writeErr(w, http.StatusServiceUnavailable, "starting and stopping is not available here")
		return
	}
	var in struct{ Action string }
	if !readJSON(w, r, &in) {
		return
	}
	switch in.Action {
	case "start":
		if err := writeWant(a.control, s.ID, "run"); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	case "stop":
		if err := writeWant(a.control, s.ID, "stop"); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	case "restart":
		if err := writeWant(a.control, s.ID, "run"); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Stop the running process. start.sh notices and starts it again after a few seconds.
		if b, err := os.ReadFile(filepath.Join(a.control, "pid-"+strconv.Itoa(s.ID))); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pid > 1 {
				syscall.Kill(pid, syscall.SIGTERM)
			}
		}
	default:
		writeErr(w, http.StatusBadRequest, "action must be start, stop or restart")
		return
	}
	go func() { time.Sleep(3 * time.Second); s.Poll() }()
	okJSON(w, nil)
}

// writeWant writes what a server should be doing, so that it is never half-written.
func writeWant(dir string, id int, want string) error {
	file := filepath.Join(dir, "want-"+strconv.Itoa(id))
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, []byte(want+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

// ---- actions ---------------------------------------------------------------------------

func (a *App) handleKick(w http.ResponseWriter, r *http.Request) {
	s := a.server(w, r)
	if s == nil {
		return
	}
	var in struct{ Name string }
	if !readJSON(w, r, &in) {
		return
	}
	name, err := word(in.Name, 64)
	if err != nil || name == "" {
		writeErr(w, http.StatusBadRequest, "invalid player name")
		return
	}
	if out, ok := a.run(w, s, "admin.kickPlayer", name); ok {
		okJSON(w, out)
	}
}

func (a *App) handleBan(w http.ResponseWriter, r *http.Request) {
	s := a.server(w, r)
	if s == nil {
		return
	}
	var in struct {
		Name    string
		Kind    string // perm, round or seconds
		Seconds int
	}
	if !readJSON(w, r, &in) {
		return
	}
	name, err := word(in.Name, 64)
	if err != nil || name == "" {
		writeErr(w, http.StatusBadRequest, "invalid player name")
		return
	}
	words := []string{"banList.add", "name", name}
	switch in.Kind {
	case "perm", "round":
		words = append(words, in.Kind)
	case "seconds":
		if in.Seconds < 1 || in.Seconds > 31536000 {
			writeErr(w, http.StatusBadRequest, "seconds must be between 1 and 31536000")
			return
		}
		words = append(words, "seconds", strconv.Itoa(in.Seconds))
	default:
		writeErr(w, http.StatusBadRequest, "kind must be perm, round or seconds")
		return
	}
	out, ok := a.run(w, s, words...)
	if !ok {
		return
	}
	s.Rcon.Do("banList.save") // keep the ban list on disk; harmless if the server does not know it
	okJSON(w, out)
}

func (a *App) handleMove(w http.ResponseWriter, r *http.Request) {
	s := a.server(w, r)
	if s == nil {
		return
	}
	var in struct {
		Name        string
		Team, Squad int
	}
	if !readJSON(w, r, &in) {
		return
	}
	name, err := word(in.Name, 64)
	if err != nil || name == "" || in.Team < 0 || in.Team > 8 || in.Squad < 0 || in.Squad > 8 {
		writeErr(w, http.StatusBadRequest, "invalid player, team or squad")
		return
	}
	if out, ok := a.run(w, s, "admin.movePlayer", name, strconv.Itoa(in.Team), strconv.Itoa(in.Squad), "true"); ok {
		okJSON(w, out)
	}
}

func (a *App) handleRound(w http.ResponseWriter, r *http.Request) {
	s := a.server(w, r)
	if s == nil {
		return
	}
	var in struct {
		Action string // next, restart or end
		Team   int    // winning team, for "end"
	}
	if !readJSON(w, r, &in) {
		return
	}
	var words []string
	switch in.Action {
	case "next":
		words = []string{"admin.runNextRound"}
	case "restart":
		words = []string{"admin.restartRound"}
	case "end":
		if in.Team < 1 || in.Team > 8 {
			writeErr(w, http.StatusBadRequest, "team must be 1 or 2")
			return
		}
		words = []string{"admin.endRound", strconv.Itoa(in.Team)}
	default:
		writeErr(w, http.StatusBadRequest, "action must be next, restart or end")
		return
	}
	if out, ok := a.run(w, s, words...); ok {
		okJSON(w, out)
	}
}

func (a *App) handleSetting(w http.ResponseWriter, r *http.Request) {
	s := a.server(w, r)
	if s == nil {
		return
	}
	var in struct{ Name, Value string }
	if !readJSON(w, r, &in) {
		return
	}
	var def *settingDef
	for i := range settingDefs {
		if settingDefs[i].Name == in.Name {
			def = &settingDefs[i]
		}
	}
	if def == nil {
		writeErr(w, http.StatusBadRequest, "unknown setting")
		return
	}
	value := in.Value
	switch def.Kind {
	case "bool":
		if value != "true" && value != "false" {
			writeErr(w, http.StatusBadRequest, "value must be true or false")
			return
		}
	case "int":
		if n, err := strconv.Atoi(value); err != nil || n < 0 || len(value) > 9 {
			writeErr(w, http.StatusBadRequest, "value must be a whole number")
			return
		}
	case "password":
		for _, r := range value {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
				writeErr(w, http.StatusBadRequest, "a password may only contain letters and digits")
				return
			}
		}
		if len(value) > def.Max {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("a password may have at most %d characters", def.Max))
			return
		}
	default: // text
		value = strings.NewReplacer("\r\n", "|", "\n", "|", "\r", "|").Replace(value) // the game uses | for line breaks
		if _, err := word(value, def.Max); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if out, ok := a.run(w, s, "vars."+def.Name, value); ok {
		okJSON(w, out)
	}
}

func (a *App) handleConsole(w http.ResponseWriter, r *http.Request) {
	s := a.server(w, r)
	if s == nil {
		return
	}
	var in struct{ Command string }
	if !readJSON(w, r, &in) {
		return
	}
	words, err := splitCommand(in.Command)
	if err != nil || len(words) == 0 {
		writeErr(w, http.StatusBadRequest, "empty or invalid command")
		return
	}
	if out, ok := a.run(w, s, words...); ok {
		okJSON(w, out)
	}
}

// splitCommand splits a typed command into words. "Double quotes" keep spaces together.
func splitCommand(s string) ([]string, error) {
	if len(s) > 1000 {
		return nil, errors.New("too long")
	}
	var words []string
	var cur strings.Builder
	inQuote, have := false, false
	flush := func() {
		if have {
			words = append(words, cur.String())
			cur.Reset()
			have = false
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			have = true
		case (r == ' ' || r == '\t') && !inQuote:
			flush()
		case r < 0x20 || r == 0x7f:
			return nil, errors.New("control character")
		default:
			cur.WriteRune(r)
			have = true
		}
	}
	if inQuote {
		return nil, errors.New("unclosed quote")
	}
	flush()
	if len(words) > 32 {
		return nil, errors.New("too many words")
	}
	return words, nil
}
