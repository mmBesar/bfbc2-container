// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 mmBesar

package main

// Pictures of the maps.
//
// The pictures are NOT part of this program or of the container image. They
// belong to the game (EA / DICE) and are published in the PRoCon project
// (github.com/AdKats/Procon-1). The first time a picture is needed, this
// program downloads it from there, once, and keeps it in /data/cache/maps.
// If the download is not possible the page simply shows no picture.
//
// The table maps each level name to its display name and its picture file
// (the picture file names were taken from the PRoCon repository, and the
// display names from The-May's bfbc2-webcon).

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// A fixed commit of the PRoCon repository, so the pictures never change under us.
const defaultMapImageBase = "https://raw.githubusercontent.com/AdKats/Procon-1/b0d3c070a2938aacc3f940facabc4b7c1bec5e39/src/Resources/Archive/Media/BFBC2/Maps/AlphaPack/"

type levelInfo struct {
	Name  string // for people
	Image string // path of the picture inside the PRoCon folder
}

var levels = map[string]levelInfo{
	"mp_002":              {"Valparaíso", "Rush/Valapariso_rush_map.jpg"},
	"mp_004":              {"Isla Inocentes", "Rush/IslaInocentes_rush_map.jpg"},
	"mp_005gr":            {"Atacama Desert", "Rush/AtacamaDesert_rush_map.jpg"},
	"mp_006":              {"Arica Harbor", "Rush/AricaHarbor_rush_map.jpg"},
	"mp_007gr":            {"White Pass", "Rush/WhitePass_rush_map.jpg"},
	"mp_008":              {"Nelson Bay", "Rush/NelsonBay_rush_map.jpg"},
	"mp_009gr":            {"Laguna Presa", "Rush/LagunaPresa_rush_map.jpg"},
	"mp_012gr":            {"Port Valdez", "Rush/PortValdez_rush_map.jpg"},
	"bc1_oasis_gr":        {"Oasis", "Rush/bc1_oasis_rush_map.jpg"},
	"bc1_harvest_day_gr":  {"Harvest Day", "Rush/bc1_harvest_day_rush_map.jpg"},
	"mp_sp_002gr":         {"Cold War", "Rush/mp_sp_002_cold_war_rush_map.jpg"},
	"mp_001":              {"Panama Canal", "Conquest/PanamaCanal_cq_map.jpg"},
	"mp_003":              {"Laguna Alta", "Conquest/LagunaAlta_cq_map.jpg"},
	"mp_005":              {"Atacama Desert", "Conquest/AtacamaDesert_cq_map.jpg"},
	"mp_006cq":            {"Arica Harbor", "Conquest/AricaHarbor_cq_map.jpg"},
	"mp_007":              {"White Pass", "Conquest/WhitePass_cq_map.jpg"},
	"mp_008cq":            {"Nelson Bay", "Conquest/NelsonBay_cq_map.jpg"},
	"mp_009cq":            {"Laguna Presa", "Conquest/LagunaPresa_cq_map.jpg"},
	"mp_012cq":            {"Port Valdez", "Conquest/PortValdez_cq_map.jpg"},
	"bc1_oasis_cq":        {"Oasis", "Conquest/bc1_oasis_cq_map.jpg"},
	"bc1_harvest_day_cq":  {"Harvest Day", "Conquest/bc1_harvest_day_cq_map.jpg"},
	"mp_sp_005cq":         {"Heavy Metal", "Conquest/HeavyMetal_cq_map.jpg"},
	"mp_001sr":            {"Panama Canal", "SquadRush/mp_001sr.jpg"},
	"mp_002sr":            {"Valparaíso", "SquadRush/mp_002sr.jpg"},
	"mp_003sr":            {"Laguna Alta", "SquadRush/LagunaAlta_map.jpg"},
	"mp_005sr":            {"Atacama Desert", "SquadRush/mp_005sr.jpg"},
	"mp_009sr":            {"Laguna Presa", "SquadRush/LagunaPresa_map.jpg"},
	"mp_012sr":            {"Port Valdez", "SquadRush/PortValdez_map.jpg"},
	"bc1_oasis_sr":        {"Oasis", "SquadRush/bc1_oasis_sr.jpg"},
	"bc1_harvest_day_sr":  {"Harvest Day", "SquadRush/bc1_harvest_day_sr.jpg"},
	"mp_sp_002sr":         {"Cold War", "SquadRush/mp_sp_002_cold_war_sr.jpg"},
	"mp_001sdm":           {"Panama Canal", "SquadDeathmatch/PanamaCanal_sqdm_map.jpg"},
	"mp_004sdm":           {"Isla Inocentes", "SquadDeathmatch/IslaInocentes_sqdm_map.jpg"},
	"mp_006sdm":           {"Arica Harbor", "SquadDeathmatch/AricaHarbor_sqdm_map.jpg"},
	"mp_007sdm":           {"White Pass", "SquadDeathmatch/WhitePass_sqdm_map.jpg"},
	"mp_008sdm":           {"Nelson Bay", "SquadDeathmatch/NelsonBay_sqdm_map.jpg"},
	"mp_009sdm":           {"Laguna Presa", "SquadDeathmatch/LagunaPresa_sqdm_map.jpg"},
	"bc1_oasis_sdm":       {"Oasis", "SquadDeathmatch/bc1_oasis_sdm.jpg"},
	"bc1_harvest_day_sdm": {"Harvest Day", "SquadDeathmatch/bc1_harvest_day_sdm.jpg"},
	"mp_sp_002sdm":        {"Cold War", "SquadDeathmatch/mp_sp_002sdm.jpg"},
	"mp_sp_005sdm":        {"Heavy Metal", "SquadDeathmatch/mp_sp_005sdm.jpg"},
	"nam_mp_002cq":        {"Vantage Point", "Conquest/nam_mp_002cq.jpg"},
	"nam_mp_003cq":        {"Hill 137", "Conquest/nam_mp_003cq.jpg"},
	"nam_mp_005cq":        {"Cao Son Temple", "Conquest/nam_mp_005cq.jpg"},
	"nam_mp_006cq":        {"Phu Bai Valley", "Conquest/nam_mp_006cq.jpg"},
	"nam_mp_007cq":        {"Operation Hastings", "Conquest/nam_mp_007cq.jpg"},
	"nam_mp_002r":         {"Vantage Point", "Rush/nam_mp_002r.jpg"},
	"nam_mp_003r":         {"Hill 137", "Rush/nam_mp_003r.jpg"},
	"nam_mp_005r":         {"Cao Son Temple", "Rush/nam_mp_005r.jpg"},
	"nam_mp_006r":         {"Phu Bai Valley", "Rush/nam_mp_006r.jpg"},
	"nam_mp_007r":         {"Operation Hastings", "Rush/nam_mp_007r.jpg"},
	"nam_mp_002sr":        {"Vantage Point", "SquadRush/nam_mp_002sr.jpg"},
	"nam_mp_003sr":        {"Hill 137", "SquadRush/nam_mp_003sr.jpg"},
	"nam_mp_005sr":        {"Cao Son Temple", "SquadRush/nam_mp_005sr.jpg"},
	"nam_mp_006sr":        {"Phu Bai Valley", "SquadRush/nam_mp_006sr.jpg"},
	"nam_mp_007sr":        {"Operation Hastings", "SquadRush/nam_mp_007sr.jpg"},
	"nam_mp_002sdm":       {"Vantage Point", "SquadDeathmatch/nam_mp_002sdm.jpg"},
	"nam_mp_003sdm":       {"Hill 137", "SquadDeathmatch/nam_mp_003sdm.jpg"},
	"nam_mp_005sdm":       {"Cao Son Temple", "SquadDeathmatch/nam_mp_005sdm.jpg"},
	"nam_mp_006sdm":       {"Phu Bai Valley", "SquadDeathmatch/nam_mp_006sdm.jpg"},
	"nam_mp_007sdm":       {"Operation Hastings", "SquadDeathmatch/nam_mp_007sdm.jpg"},
}

// normalizeLevel turns "Levels/MP_002" into "mp_002".
func normalizeLevel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.TrimPrefix(s, "levels/")
}

// MapImages finds map pictures on disk, and downloads missing ones if allowed.
type MapImages struct {
	dir      string
	baseURL  string
	download bool
	client   *http.Client

	mu     sync.Mutex
	failed map[string]time.Time // pictures that could not be downloaded, and when to try again
}

func NewMapImages(dir, baseURL string, download bool) *MapImages {
	if baseURL == "" {
		baseURL = defaultMapImageBase
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	return &MapImages{
		dir: dir, baseURL: baseURL, download: download,
		client: &http.Client{Timeout: 15 * time.Second},
		failed: map[string]time.Time{},
	}
}

// Path returns the file to send for a level, downloading it first if needed.
func (m *MapImages) Path(level string) (string, error) {
	info, ok := levels[normalizeLevel(level)]
	if !ok {
		return "", errors.New("unknown map")
	}
	file := filepath.Join(m.dir, strings.ReplaceAll(info.Image, "/", "_"))

	m.mu.Lock()
	defer m.mu.Unlock() // one download at a time keeps things simple

	if _, err := os.Stat(file); err == nil {
		return file, nil
	}
	if !m.download {
		return "", errors.New("picture not available (downloads are off)")
	}
	if until, bad := m.failed[file]; bad && time.Now().Before(until) {
		return "", errors.New("picture not available (download failed recently)")
	}
	if err := m.fetch(info.Image, file); err != nil {
		m.failed[file] = time.Now().Add(5 * time.Minute)
		return "", err
	}
	return file, nil
}

func (m *MapImages) fetch(rel, dst string) error {
	resp, err := m.client.Get(m.baseURL + rel)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download answered %s", resp.Status)
	}
	const limit = 3 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if len(data) > limit {
		return errors.New("picture is too big")
	}
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 || data[2] != 0xFF {
		return errors.New("download is not a JPEG picture")
	}
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return err
	}
	tmp := dst + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
