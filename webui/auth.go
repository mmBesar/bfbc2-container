// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 mmBesar

package main

// Login and protection of the web interface.
//
//   - Everything except /healthz needs the user name and password (HTTP Basic).
//   - Requests that change something must be JSON and must come from this very
//     page (same origin). That stops other websites from making your browser
//     click buttons here.

import (
	"crypto/rand"
	"crypto/subtle"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// loadCredentials returns the user name and password from the environment.
// If no password is set, a random one is made once and kept in a file.
func loadCredentials(configDir string) (user, pass string) {
	user = getenv("WEB_USER", "admin")
	pass = os.Getenv("WEB_PASSWORD")
	if pass != "" {
		return user, pass
	}
	file := filepath.Join(configDir, "web-password")
	if b, err := os.ReadFile(file); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		return user, strings.TrimSpace(string(b))
	}
	pass = randomString(16)
	if err := os.MkdirAll(configDir, 0o755); err == nil {
		if err := os.WriteFile(file, []byte(pass+"\n"), 0o600); err == nil {
			log.Printf("WEB_PASSWORD is not set. Generated a random one and saved it in %s (user name: %s)", file, user)
			return user, pass
		}
	}
	log.Printf("WEB_PASSWORD is not set and no password file could be written. Using a random password for this run only: %s", pass)
	return user, pass
}

func randomString(n int) string {
	const alphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	out := make([]byte, n)
	for i := range out {
		k, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			panic(err) // no randomness available: better to stop than to be predictable
		}
		out[i] = alphabet[k.Int64()]
	}
	return string(out)
}

func (a *App) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")

		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}

		user, pass, ok := r.BasicAuth()
		userOK := subtle.ConstantTimeCompare([]byte(user), []byte(a.user)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(a.pass)) == 1
		if !ok || !userOK || !passOK {
			time.Sleep(300 * time.Millisecond) // slows down password guessing
			h.Set("WWW-Authenticate", `Basic realm="BFBC2 Server Manager", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				http.Error(w, "JSON required", http.StatusUnsupportedMediaType)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host {
					http.Error(w, "Cross-site request refused", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
