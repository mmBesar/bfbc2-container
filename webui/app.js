// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 mmBesar
"use strict";

// The web page of the BFBC2 Server Manager. No frameworks, no build step.
// Everything that comes from a server (player names, server names, ...) is put
// on the page as plain text, never as HTML, so a nasty player name cannot do harm.

const TYPE_NAMES = {
  rush: "Rush", conq: "Conquest", conquest: "Conquest", sqdm: "Squad Deathmatch", sqrush: "Squad Rush",
  vietrush: "Vietnam Rush", vietconq: "Vietnam Conquest", vietsqdm: "Vietnam Squad Deathmatch", vietsqrush: "Vietnam Squad Rush",
};

// Display names of all maps, by level name (from The-May's bfbc2-webcon).
const LEVEL_NAMES = {
  "mp_002": "Valparaíso",
  "mp_004": "Isla Inocentes",
  "mp_005gr": "Atacama Desert",
  "mp_006": "Arica Harbor",
  "mp_007gr": "White Pass",
  "mp_008": "Nelson Bay",
  "mp_009gr": "Laguna Presa",
  "mp_012gr": "Port Valdez",
  "bc1_oasis_gr": "Oasis",
  "bc1_harvest_day_gr": "Harvest Day",
  "mp_sp_002gr": "Cold War",
  "mp_001": "Panama Canal",
  "mp_003": "Laguna Alta",
  "mp_005": "Atacama Desert",
  "mp_006cq": "Arica Harbor",
  "mp_007": "White Pass",
  "mp_008cq": "Nelson Bay",
  "mp_009cq": "Laguna Presa",
  "mp_012cq": "Port Valdez",
  "bc1_oasis_cq": "Oasis",
  "bc1_harvest_day_cq": "Harvest Day",
  "mp_sp_005cq": "Heavy Metal",
  "mp_001sr": "Panama Canal",
  "mp_002sr": "Valparaíso",
  "mp_003sr": "Laguna Alta",
  "mp_005sr": "Atacama Desert",
  "mp_009sr": "Laguna Presa",
  "mp_012sr": "Port Valdez",
  "bc1_oasis_sr": "Oasis",
  "bc1_harvest_day_sr": "Harvest Day",
  "mp_sp_002sr": "Cold War",
  "mp_001sdm": "Panama Canal",
  "mp_004sdm": "Isla Inocentes",
  "mp_006sdm": "Arica Harbor",
  "mp_007sdm": "White Pass",
  "mp_008sdm": "Nelson Bay",
  "mp_009sdm": "Laguna Presa",
  "bc1_oasis_sdm": "Oasis",
  "bc1_harvest_day_sdm": "Harvest Day",
  "mp_sp_002sdm": "Cold War",
  "mp_sp_005sdm": "Heavy Metal",
  "nam_mp_002cq": "Vantage Point",
  "nam_mp_003cq": "Hill 137",
  "nam_mp_005cq": "Cao Son Temple",
  "nam_mp_006cq": "Phu Bai Valley",
  "nam_mp_007cq": "Operation Hastings",
  "nam_mp_002r": "Vantage Point",
  "nam_mp_003r": "Hill 137",
  "nam_mp_005r": "Cao Son Temple",
  "nam_mp_006r": "Phu Bai Valley",
  "nam_mp_007r": "Operation Hastings",
  "nam_mp_002sr": "Vantage Point",
  "nam_mp_003sr": "Hill 137",
  "nam_mp_005sr": "Cao Son Temple",
  "nam_mp_006sr": "Phu Bai Valley",
  "nam_mp_007sr": "Operation Hastings",
  "nam_mp_002sdm": "Vantage Point",
  "nam_mp_003sdm": "Hill 137",
  "nam_mp_005sdm": "Cao Son Temple",
  "nam_mp_006sdm": "Phu Bai Valley",
  "nam_mp_007sdm": "Operation Hastings",
};

const SQUADS = ["No squad", "Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Golf", "Hotel"];

const state = { servers: [], selected: null, tab: "players", mapImages: false };

// ---- tiny helpers -----------------------------------------------------------

const $ = (selector, root = document) => root.querySelector(selector);

// el("div", {class: "x", onclick: fn}, "text", otherElement) builds an element.
function el(tag, attrs = {}, ...kids) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === false || v == null) continue;
    if (k.startsWith("on")) e.addEventListener(k.slice(2), v);
    else if (v === true) e.setAttribute(k, "");
    else e.setAttribute(k, v);
  }
  for (const kid of kids.flat()) {
    if (kid == null || kid === false) continue;
    e.append(kid instanceof Node ? kid : document.createTextNode(String(kid)));
  }
  return e;
}

let toastTimer = null;
function toast(message, bad = false) {
  const t = $("#toast");
  t.textContent = message;
  t.className = bad ? "bad" : "";
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { t.hidden = true; }, 4000);
}

async function api(method, path, body) {
  const options = { method, headers: {} };
  if (body !== undefined) {
    options.headers["Content-Type"] = "application/json";
    options.body = JSON.stringify(body);
  }
  const response = await fetch(path, options);
  let data = {};
  try { data = await response.json(); } catch (_) { /* not JSON */ }
  if (!response.ok) throw new Error(data.error || `${response.status} ${response.statusText}`);
  return data;
}

function levelId(level) {
  return (level || "").toLowerCase().replace(/^levels\//, "");
}

// "Levels/MP_002" -> "Valparaíso". Unknown maps are shown by their id.
function mapLabel(level) {
  if (!level) return "-";
  const id = levelId(level);
  return LEVEL_NAMES[id] || id;
}

// A coloured tile with the map's name. Every map gets its own colour (the same
// in all modes), so the page looks good with no pictures at all. If map pictures
// are available they are shown on top of the tile.
// (The colour is set through .style, not a style attribute: the page's security
// rules do not allow inline style attributes.)
function mapVisual(level, cls) {
  const id = levelId(level);
  const base = id.replace(/_?(gr|cq|sr|sdm|r)$/, "");
  let hash = 0;
  for (const c of base) hash = (hash * 31 + c.charCodeAt(0)) >>> 0;
  const hue = hash % 360;
  const tile = el("div", { class: "tile " + cls, title: id },
    cls === "mini" ? null : el("span", { class: "tile-name" }, mapLabel(level)));
  tile.style.background = `linear-gradient(135deg, hsl(${hue} 42% 32%), hsl(${(hue + 50) % 360} 46% 16%))`;
  if (state.mapImages && id in LEVEL_NAMES) {
    tile.append(el("img", { src: `/maps/${id}.jpg`, alt: "", loading: "lazy", onerror: (e) => e.target.remove() }));
  }
  return tile;
}

const serverById = (id) => state.servers.find((s) => s.id === id);
const modeName = (s) => TYPE_NAMES[s.type] || s.type;

// ---- overview ---------------------------------------------------------------

function renderCards() {
  const box = $("#cards");
  box.replaceChildren();
  if (state.servers.length === 0) {
    box.append(el("p", { class: "muted" }, "No game servers are configured. Set SERVER_1_TYPE in docker-compose.yml."));
    return;
  }
  for (const s of state.servers) {
    const info = s.info;
    const stopped = s.state === "stopped";
    box.append(el("article", {
      class: "card" + (s.online ? "" : " off"), tabindex: "0", role: "button",
      onclick: () => select(s.id),
      onkeydown: (e) => { if (e.key === "Enter") select(s.id); },
    },
      el("div", { class: "card-top" },
        el("strong", {}, info ? info.name : `Server ${s.id}`),
        stopped
          ? el("span", { class: "pill" }, "stopped")
          : el("span", { class: "pill " + (s.online ? "ok" : "bad") }, s.online ? "online" : "offline")),
      el("div", { class: "muted" }, modeName(s)),
      s.online && info ? mapVisual(info.map, "thumb") : null,
      s.online && info
        ? [el("div", {}, mapLabel(info.map)),
           el("div", { class: "big" }, `${info.players}/${info.maxPlayers}`, el("small", {}, " players"))]
        : el("div", { class: "muted" }, stopped ? "Not running. Open it to start it." : (s.error || "waiting for the server...")),
      el("div", { class: "card-foot" },
        el("span", { class: "muted small" }, `game port ${s.gamePort}`),
        stopped
          ? el("button", { type: "button", class: "small", onclick: (e) => { e.stopPropagation(); startServer(s); } }, "Start")
          : el("button", { type: "button", class: "small danger", onclick: (e) => { e.stopPropagation(); stopServer(s); } }, "Stop"))));
  }
  $("#bulk").hidden = state.servers.length === 0;
}

// ---- one server ---------------------------------------------------------------

function select(id) {
  state.selected = id;
  state.tab = "players";
  $("#overview").hidden = true;
  $("#detail").hidden = false;
  $("#console-out").textContent = "";
  showTab("players");
  renderDetail();
}

function back() {
  state.selected = null;
  $("#detail").hidden = true;
  $("#overview").hidden = false;
}

function showTab(name) {
  state.tab = name;
  for (const b of document.querySelectorAll("#tabs button")) b.classList.toggle("active", b.dataset.tab === name);
  for (const t of document.querySelectorAll(".tab")) t.hidden = t.id !== "tab-" + name;
  if (name === "settings" || name === "round") loadSettings();
}

// Updates the parts of the detail view that change every few seconds.
function renderDetail() {
  const s = serverById(state.selected);
  if (!s) return;
  const info = s.info;
  $("#d-title").textContent = info ? info.name : `Server ${s.id}`;
  const stopped = s.state === "stopped";
  const pill = $("#d-state");
  pill.textContent = stopped ? "stopped" : (s.online ? "online" : "offline");
  pill.className = "pill " + (stopped ? "" : (s.online ? "ok" : "bad"));
  $("#p-start").hidden = !stopped;
  $("#p-stop").hidden = stopped;
  $("#p-restart").hidden = stopped;
  $("#d-sub").textContent = stopped
    ? "This server is stopped. It uses no CPU or memory and is not in the server list. It stays stopped until you start it or the container restarts."
    : s.online && info
    ? `${modeName(s)} | ${mapLabel(info.map)} | round ${info.roundsPlayed} of ${info.roundsTotal} | ` +
      `${info.players}/${info.maxPlayers} players | ${info.ranked ? "ranked" : "unranked"}` +
      `${info.hasPassword ? " | password protected" : ""} | game port ${s.gamePort}`
    : (s.error || "waiting for the server...");
  $("#round-now").textContent = info ? `Now playing: ${mapLabel(info.map)}` : "";
  const hero = $("#d-image");
  const level = info && s.online ? levelId(info.map) : "";
  if (!(level in LEVEL_NAMES)) {
    hero.hidden = true;
    hero.dataset.level = "";
    hero.replaceChildren();
  } else if (hero.dataset.level !== level) {
    hero.dataset.level = level;
    hero.replaceChildren(mapVisual(level, "hero"));
    hero.hidden = false;
  }
  renderPlayers(s);
}

function renderPlayers(s) {
  const body = $("#players tbody");
  body.replaceChildren();
  const players = s.players || [];
  $("#no-players").hidden = players.length > 0;
  $("#players").hidden = players.length === 0;
  for (const p of players) {
    const name = p.name || "";
    const team = parseInt(p.teamId, 10) || 0;
    const squad = parseInt(p.squadId, 10) || 0;
    body.append(el("tr", {},
      el("td", { class: "team-" + team }, p.clanTag ? `[${p.clanTag}] ${name}` : name),
      el("td", {}, team === 0 ? "-" : `Team ${team}`),
      el("td", {}, SQUADS[squad] || squad),
      el("td", {}, p.kills || "0"), el("td", {}, p.deaths || "0"),
      el("td", {}, p.score || "0"), el("td", {}, p.ping || "-"),
      el("td", { class: "actions" },
        el("button", { type: "button", class: "warn", onclick: () => kick(name) }, "Kick"),
        el("button", { type: "button", class: "danger", onclick: () => ban(name) }, "Ban"),
        el("button", { type: "button", onclick: () => moveTeam(name, team) }, "Other team"),
        el("button", { type: "button", onclick: () => moveSquad(name, team) }, "Squad..."))));
  }
}

// Sends a command to one server and reports the result.
async function actOn(id, action, body, okMessage) {
  try {
    await api("POST", `/api/servers/${id}/${action}`, body);
    toast(okMessage);
    setTimeout(refresh, 700);
    return true;
  } catch (e) {
    toast(e.message, true);
    return false;
  }
}

// The same, for the server that is open in the detail view.
const act = (action, body, okMessage) => actOn(state.selected, action, body, okMessage);

// Start or stop a server. Used by the buttons on the server list and in the detail view.
function startServer(s) {
  return actOn(s.id, "power", { action: "start" }, `Starting ${s.info ? s.info.name : "server " + s.id}...`);
}

function stopServer(s) {
  const name = s.info ? s.info.name : `server ${s.id}`;
  const players = s.info && s.info.players ? ` ${s.info.players} player(s) are on it.` : "";
  if (confirm(`Stop ${name}?${players} They will be disconnected.`)) {
    return actOn(s.id, "power", { action: "stop" }, `Stopping ${name}...`);
  }
}

function kick(name) {
  if (confirm(`Kick ${name}?`)) act("kick", { name }, `Kicked ${name}`);
}

function ban(name) {
  const answer = prompt(`Ban ${name}.\nType "perm" for a permanent ban, "round" until the round ends, or a number of seconds:`, "perm");
  if (answer === null) return;
  const text = answer.trim().toLowerCase();
  if (text === "perm" || text === "round") return void act("ban", { name, kind: text }, `Banned ${name} (${text})`);
  const seconds = parseInt(text, 10);
  if (!(seconds > 0)) return void toast("Please type perm, round, or a number of seconds.", true);
  act("ban", { name, kind: "seconds", seconds }, `Banned ${name} for ${seconds} seconds`);
}

function moveTeam(name, team) {
  const other = team === 1 ? 2 : 1;
  if (confirm(`Move ${name} to team ${other}?`)) act("move", { name, team: other, squad: 0 }, `Moved ${name} to team ${other}`);
}

function moveSquad(name, team) {
  const answer = prompt(`Squad for ${name} (0 = none, 1 = Alpha, 2 = Bravo, ... 8 = Hotel):`, "1");
  if (answer === null) return;
  const squad = parseInt(answer, 10);
  if (!(squad >= 0 && squad <= 8)) return void toast("Please type a number from 0 to 8.", true);
  act("move", { name, team, squad }, `Moved ${name} to ${SQUADS[squad]}`);
}

function endRound(team) {
  if (confirm(`End the round with team ${team} winning?`)) act("round", { action: "end", team }, "Round ended");
}

// ---- settings and map list ------------------------------------------------------

async function loadSettings() {
  const form = $("#settings-form");
  form.replaceChildren(el("p", { class: "muted" }, "Loading..."));
  try {
    const data = await api("GET", `/api/servers/${state.selected}/settings`);
    buildSettings(data);
    buildMapList(data);
  } catch (e) {
    form.replaceChildren(el("p", { class: "err" }, e.message));
  }
}

async function saveSetting(name, value, undo) {
  try {
    await api("POST", `/api/servers/${state.selected}/setting`, { name, value });
    toast(`Saved ${name}`);
    setTimeout(refresh, 700);
  } catch (e) {
    toast(e.message, true);
    if (undo) undo();
  }
}

function buildSettings(data) {
  const form = $("#settings-form");
  form.replaceChildren();
  for (const def of data.defs) {
    if (!(def.name in data.values)) continue;
    const value = data.values[def.name];
    let control;
    if (def.kind === "bool") {
      const box = el("input", { type: "checkbox", id: "set-" + def.name });
      box.checked = value === "true";
      box.addEventListener("change", () => saveSetting(def.name, box.checked ? "true" : "false", () => { box.checked = !box.checked; }));
      control = box;
    } else {
      const input = el("input", {
        type: def.kind === "int" ? "number" : "text", id: "set-" + def.name,
        value, maxlength: def.max || false, min: def.kind === "int" ? "0" : false,
      });
      const save = el("button", { type: "button", onclick: () => saveSetting(def.name, input.value) }, "Save");
      control = [input, save];
    }
    form.append(el("div", { class: "setting" }, el("label", { for: "set-" + def.name }, def.name), control));
  }
}

function buildMapList(data) {
  const list = $("#maplist");
  list.replaceChildren();
  const current = (data.currentLevel || "").toLowerCase();
  for (const level of data.maps) {
    const now = level.toLowerCase() === current;
    list.append(el("li", { class: now ? "current" : "", title: level },
      mapVisual(level, "mini"), " ", mapLabel(level), now ? "  <- now" : ""));
  }
  if (data.maps.length === 0) list.append(el("li", { class: "muted" }, "The server reported no maps."));
}

// ---- console -----------------------------------------------------------------------

async function sendConsole(event) {
  event.preventDefault();
  const input = $("#console-in");
  const command = input.value.trim();
  if (!command) return;
  const out = $("#console-out");
  out.textContent += `> ${command}\n`;
  try {
    const data = await api("POST", `/api/servers/${state.selected}/console`, { command });
    out.textContent += (data.output.length ? data.output.join(" ") : "OK") + "\n\n";
  } catch (e) {
    out.textContent += `ERROR: ${e.message}\n\n`;
  }
  out.scrollTop = out.scrollHeight;
  input.value = "";
}

// ---- refreshing ------------------------------------------------------------------------

async function refresh() {
  try {
    state.servers = await api("GET", "/api/servers");
    $("#stamp").textContent = "updated " + new Date().toLocaleTimeString();
    renderCards();
    renderDetail();
  } catch (e) {
    $("#stamp").textContent = "cannot reach the manager: " + e.message;
  }
}

async function loadStatus() {
  const pill = $("#master");
  try {
    const st = await api("GET", "/api/status");
    const ok = st.master.reachable;
    pill.textContent = ok ? "master: running" : "master: not reachable";
    pill.className = "pill " + (ok ? "ok" : "bad");
    pill.title = st.master.ports.map((p) => `${p.name} (port ${p.port}): ${p.open ? "open" : "closed"}`).join("\n");
    $("#version").textContent = "v" + st.version;
    if (state.mapImages !== !!st.mapImages) {
      state.mapImages = !!st.mapImages;
      $("#d-image").dataset.level = "";   // draw the tiles again, now with or without pictures
      renderCards();
      renderDetail();
    }
  } catch (_) {
    pill.textContent = "master: unknown";
    pill.className = "pill";
  }
}

// ---- start ---------------------------------------------------------------------------------

$("#p-start").addEventListener("click", () => startServer(serverById(state.selected)));
$("#p-stop").addEventListener("click", () => stopServer(serverById(state.selected)));
$("#p-restart").addEventListener("click", () => {
  if (confirm("Restart this server? Players on it will be disconnected.")) act("power", { action: "restart" }, "Restarting the server...");
});
$("#bulk-start").addEventListener("click", async () => {
  const stopped = state.servers.filter((s) => s.state === "stopped");
  if (stopped.length === 0) return void toast("No server is stopped.");
  for (const s of stopped) await startServer(s);
});
$("#bulk-stop").addEventListener("click", async () => {
  const running = state.servers.filter((s) => s.state !== "stopped");
  if (running.length === 0) return void toast("No server is running.");
  const players = running.reduce((sum, s) => sum + (s.info ? s.info.players : 0), 0);
  if (!confirm(`Stop ${running.length} server(s)? ${players} player(s) are on them. They will be disconnected.`)) return;
  for (const s of running) await actOn(s.id, "power", { action: "stop" }, `Stopping ${s.info ? s.info.name : "server " + s.id}...`);
});
$("#back").addEventListener("click", back);
for (const b of document.querySelectorAll("#tabs button")) b.addEventListener("click", () => showTab(b.dataset.tab));
$("#r-next").addEventListener("click", () => { if (confirm("Start the next round?")) act("round", { action: "next" }, "Starting the next round"); });
$("#r-restart").addEventListener("click", () => { if (confirm("Restart the current round?")) act("round", { action: "restart" }, "Restarting the round"); });
$("#r-end1").addEventListener("click", () => endRound(1));
$("#r-end2").addEventListener("click", () => endRound(2));
$("#console-form").addEventListener("submit", sendConsole);

refresh();
loadStatus();
setInterval(refresh, 4000);
setInterval(loadStatus, 10000);
