// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 mmBesar

// bfbc2-webui: the web interface of the BFBC2 all-in-one container.
//
// It runs next to the game servers, talks to each one over RCON on localhost,
// and shows their status with buttons for the common admin tasks.
package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The web page: three plain files that live next to the Go code.
//
//go:embed index.html app.js style.css
var webFiles embed.FS

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("[webui] ")
	if len(os.Args) > 1 && (os.Args[1] == "-version" || os.Args[1] == "--version") {
		fmt.Println(version)
		return
	}

	dataDir := getenv("DATA_DIR", "/data")
	instanceRoot := getenv("INSTANCE_ROOT", filepath.Join(dataDir, "instances"))
	configDir := getenv("CONFIG_DIR", filepath.Join(dataDir, "config"))
	masterDir := getenv("MASTERDIR", filepath.Join(dataDir, "master"))

	control := os.Getenv("CONTROL_DIR")
	servers := DiscoverServers(os.Environ(), instanceRoot, log.Printf)
	byID := map[int]*Server{}
	for _, s := range servers {
		s.Control = control
		byID[s.ID] = s
		s.StartPolling(5 * time.Second)
	}

	static := fs.FS(webFiles)
	user, pass := loadCredentials(configDir)
	// Map pictures are OFF by default: the container then depends on nothing outside.
	downloads := strings.ToLower(getenv("WEB_MAP_IMAGES", "false"))
	app := &App{
		servers: servers, byID: byID, user: user, pass: pass,
		masterConf: filepath.Join(masterDir, "config.ini"),
		started:    time.Now(), static: static,
		maps: NewMapImages(filepath.Join(dataDir, "cache", "maps"), os.Getenv("WEB_MAP_IMAGE_URL"),
			downloads == "true" || downloads == "1" || downloads == "yes" || downloads == "on"),
		control: control,
	}

	addr := net.JoinHostPort(getenv("WEB_BIND", "0.0.0.0"), getenv("WEB_PORT", "5010"))
	srv := &http.Server{
		Addr:              addr,
		Handler:           app.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("version %s, managing %d server(s), listening on http://%s", version, len(servers), addr)
	log.Fatal(srv.ListenAndServe())
}
