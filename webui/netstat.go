// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 mmBesar

package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// portListening reports whether some program on this machine is waiting for
// connections on a TCP port. It reads the system's socket tables instead of
// connecting, so it never shows up in the master server's log (a connection
// that opens and closes at once makes the master log an error each time).
func portListening(port int) bool {
	return listeningIn("/proc/net/tcp", port) || listeningIn("/proc/net/tcp6", port)
}

func listeningIn(path string, port int) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Scan() // header line
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 || fields[3] != "0A" { // 0A = LISTEN
			continue
		}
		local := fields[1] // like 0100007F:1F90 (address:port, port in hex)
		i := strings.LastIndex(local, ":")
		if i < 0 {
			continue
		}
		if p, err := strconv.ParseUint(local[i+1:], 16, 32); err == nil && int(p) == port {
			return true
		}
	}
	return false
}
