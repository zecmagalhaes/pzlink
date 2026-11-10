package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/zecmagalhaes/pzlink/pzasset"
	"tailscale.com/tsnet"
)

// The host publishes the lobby roster inside the tailnet; guests send their
// profile (sex and profession) and fetch the roster. Only nicknames, pings and
// those two choices travel: every PZLink builds the 3D characters from its own
// game installation, seeded by the player's nickname.
const rosterPort = 7471

const defaultNick = "sobrevivente"

type profile struct {
	Sex        string `json:"sex"`        // "m", "f" or "r" (random)
	Profession string `json:"profession"` // game profession id, e.g. "policeofficer"
}

var professionID = regexp.MustCompile(`^[a-z_]{1,32}$`)

func cleanProfile(p profile) profile {
	if p.Sex != "m" && p.Sex != "f" {
		p.Sex = "r"
	}
	p.Profession = strings.ToLower(strings.TrimSpace(p.Profession))
	if !professionID.MatchString(p.Profession) {
		p.Profession = ""
	}
	return p
}

type rosterEntry struct {
	Name   string  `json:"name"`
	Host   bool    `json:"host,omitempty"`
	Online bool    `json:"online"`
	PingMs float64 `json:"pingMs"`
	Level  string  `json:"level"`
	Path   string  `json:"path,omitempty"`
	profile
}

type lobbyPlayer struct {
	rosterEntry
	Self           bool                `json:"self,omitempty"`
	Seed           string              `json:"seed"`
	Character      *pzasset.Character  `json:"character,omitempty"`
	ProfessionInfo *pzasset.Profession `json:"professionInfo,omitempty"`
}

type lobbyState struct {
	GameFound bool          `json:"gameFound"`
	IdleSpeed float64       `json:"idleSpeed,omitempty"`
	GameError string        `json:"gameError,omitempty"`
	Players   []lobbyPlayer `json:"players"`
}

// assets holds the game installation, found on first use.
type assets struct {
	once sync.Once
	game *pzasset.Game
	cat  *pzasset.Catalog
	err  error

	idleSpeed   float64
	professions []pzasset.Profession

	mu  sync.Mutex
	glb map[string][]byte
}

func (as *assets) load() error {
	as.once.Do(func() {
		as.game, as.err = pzasset.FindGame()
		if as.err == nil {
			as.cat, as.err = as.game.LoadCatalog()
			as.idleSpeed = as.game.IdleSpeed()
			as.professions = as.game.Professions()
		}
		if as.err != nil {
			log.Println("lobby:", as.err)
		}
		as.glb = map[string][]byte{}
	})
	return as.err
}

func (as *assets) profession(id string) *pzasset.Profession {
	for i := range as.professions {
		if as.professions[i].ID == id {
			return &as.professions[i]
		}
	}
	return nil
}

func (as *assets) character(seed uint64, p profile) *pzasset.Character {
	ch, err := as.cat.NewCharacter(seed, pzasset.Options{Sex: p.Sex, Profession: p.Profession})
	if err != nil {
		return nil
	}
	return ch
}

func (as *assets) characterGLB(seed uint64, p profile) ([]byte, error) {
	if err := as.load(); err != nil {
		return nil, err
	}
	p = cleanProfile(p)
	key := fmt.Sprintf("%d|%s|%s", seed, p.Sex, p.Profession)
	as.mu.Lock()
	if b, ok := as.glb[key]; ok {
		as.mu.Unlock()
		return b, nil
	}
	as.mu.Unlock()
	ch, err := as.cat.NewCharacter(seed, pzasset.Options{Sex: p.Sex, Profession: p.Profession})
	if err != nil {
		return nil, err
	}
	b, err := as.cat.GLB(ch)
	if err != nil {
		return nil, err
	}
	as.mu.Lock()
	if len(as.glb) >= 32 {
		as.glb = map[string][]byte{}
	}
	as.glb[key] = b
	as.mu.Unlock()
	return b, nil
}

func seedFor(name string) uint64 {
	h := fnv.New64a()
	h.Write([]byte("pzlink:" + strings.ToLower(strings.TrimSpace(name))))
	return h.Sum64()
}

// displayName turns a tailnet hostname suffix ("joao-a3") back into the nick.
var nodeSuffix = regexp.MustCompile(`-[0-9a-f]{2}$`)

func displayName(peer string) string { return nodeSuffix.ReplaceAllString(peer, "") }

func (a *app) selfNick() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg.Nick != "" {
		return a.cfg.Nick
	}
	return defaultNick
}

func (a *app) selfProfile() profile {
	a.mu.Lock()
	defer a.mu.Unlock()
	return cleanProfile(profile{Sex: a.cfg.Sex, Profession: a.cfg.Profession})
}

// hostRoster is the roster as the host sees it.
func (a *app) hostRoster() []rosterEntry {
	out := []rosterEntry{{Name: a.selfNick(), Host: true, Online: true, Level: "good", Path: "anfitrião", profile: a.selfProfile()}}
	a.mu.Lock()
	peers := append([]peerInfo(nil), a.peers...)
	profiles := make(map[string]profile, len(a.profiles))
	for k, v := range a.profiles {
		profiles[k] = v
	}
	a.mu.Unlock()
	for _, p := range peers {
		if !p.Online {
			continue
		}
		out = append(out, rosterEntry{Name: displayName(p.Name), Online: true, PingMs: p.Ping.AvgMs,
			Level: p.Ping.Level, Path: p.Ping.Path, profile: cleanProfile(profiles[p.IP])})
	}
	return out
}

// serveRoster publishes the roster on the host's tailnet address.
func (a *app) serveRoster(ctx context.Context, s *tsnet.Server, ip string) {
	ln, err := s.Listen("tcp", fmt.Sprintf("%s:%d", ip, rosterPort))
	if err != nil {
		log.Printf("lobby: não consegui publicar a lista: %v", err)
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /roster", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(a.hostRoster())
	})
	// A guest announces its choices. The sender is identified by its tailnet
	// address, which WireGuard authenticates, so nobody can set another
	// player's profile.
	mux.HandleFunc("POST /profile", func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "origem inválida", http.StatusBadRequest)
			return
		}
		var p profile
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&p); err != nil {
			http.Error(w, "perfil inválido", http.StatusBadRequest)
			return
		}
		a.setProfile(ip, p)
		w.WriteHeader(http.StatusNoContent)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second,
		WriteTimeout: 5 * time.Second, MaxHeaderBytes: 8 << 10}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	srv.Serve(ln)
}

// setProfile records a guest's profile; at most 64 devices are remembered.
func (a *app) setProfile(ip string, p profile) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.profiles == nil {
		a.profiles = map[string]profile{}
	}
	if _, known := a.profiles[ip]; known || len(a.profiles) < 64 {
		a.profiles[ip] = cleanProfile(p)
	}
}

// fetchRosterLoop sends the guest's profile and keeps a copy of the roster.
func (a *app) fetchRosterLoop(ctx context.Context, s *tsnet.Server, hostIP string) {
	client := s.HTTPClient()
	client.Timeout = 4 * time.Second
	base := fmt.Sprintf("http://%s:%d", hostIP, rosterPort)
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		sendProfile(ctx, client, base+"/profile", a.selfProfile())
		if r, err := fetchRoster(ctx, client, base+"/roster"); err == nil {
			a.mu.Lock()
			a.remoteRoster = r
			a.mu.Unlock()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func sendProfile(ctx context.Context, c *http.Client, url string, p profile) {
	b, _ := json.Marshal(p)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if resp, err := c.Do(req); err == nil {
		resp.Body.Close()
	}
}

func fetchRoster(ctx context.Context, c *http.Client, url string) ([]rosterEntry, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New(resp.Status)
	}
	var r []rosterEntry
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&r); err != nil {
		return nil, err
	}
	return cleanRoster(r), nil
}

// cleanRoster bounds what a (possibly hostile) host can make us display.
func cleanRoster(in []rosterEntry) []rosterEntry {
	var out []rosterEntry
	for _, e := range in {
		if len(out) >= 16 {
			break
		}
		e.Name = sanitizeName(e.Name)
		if e.Name == "" {
			continue
		}
		switch e.Level {
		case "good", "ok", "bad", "wait":
		default:
			e.Level = "wait"
		}
		if e.PingMs < 0 || e.PingMs > 10000 {
			e.PingMs = 0
		}
		if len(e.Path) > 40 {
			e.Path = e.Path[:40]
		}
		e.profile = cleanProfile(e.profile)
		out = append(out, e)
	}
	return out
}

func (a *app) lobby() lobbyState {
	var st lobbyState
	err := a.assets.load()
	st.GameFound = err == nil
	if err != nil {
		st.GameError = err.Error()
	} else {
		st.IdleSpeed = a.assets.idleSpeed
	}
	a.mu.Lock()
	mode, remote := a.mode, append([]rosterEntry(nil), a.remoteRoster...)
	a.mu.Unlock()
	self := a.selfNick()

	var roster []rosterEntry
	switch {
	case mode == "host":
		roster = a.hostRoster()
	case mode == "join" && len(remote) > 0:
		roster = remote
		mine := a.snapshot().HostPing
		for i := range roster {
			if strings.EqualFold(roster[i].Name, self) && !roster[i].Host && mine != nil {
				roster[i].PingMs, roster[i].Level = mine.AvgMs, mine.Level
			}
		}
	default:
		roster = []rosterEntry{{Name: self, Level: "wait", Path: "offline"}}
	}
	mine := a.selfProfile()
	for _, e := range roster {
		p := lobbyPlayer{rosterEntry: e, Seed: fmt.Sprint(seedFor(e.Name))}
		p.Self = strings.EqualFold(e.Name, self) && (e.Host == (mode == "host"))
		if mode == "" {
			p.Self = true
		}
		if p.Self {
			p.profile = mine // your own choices show up at once
		}
		a.decorate(&p, st.GameFound)
		st.Players = append(st.Players, p)
	}
	return st
}

func (a *app) decorate(p *lobbyPlayer, gameFound bool) {
	if !gameFound {
		return
	}
	p.Character = a.assets.character(seedFor(p.Name), p.profile)
	p.ProfessionInfo = a.assets.profession(p.Profession)
}

// propHandler serves a scenery model or its texture from the game folder.
func (a *app) propHandler(w http.ResponseWriter, r *http.Request) {
	if err := a.assets.load(); err != nil {
		http.Error(w, "jogo não encontrado", http.StatusNotFound)
		return
	}
	info, ok := a.assets.game.Prop(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch r.PathValue("part") {
	case "model":
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, info.Model)
	case "texture":
		if info.Texture == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		http.ServeFile(w, r, info.Texture)
	case "info":
		writeJSON(w, http.StatusOK, map[string]any{"scale": info.Scale, "hasTexture": info.Texture != ""})
	default:
		http.NotFound(w, r)
	}
}

// withDemo fills the lobby with made-up players (?demo=N), to preview the
// scene without friends connected.
func (a *app) withDemo(st lobbyState, n int) lobbyState {
	fake := []rosterEntry{
		{Name: "anfitriao", Host: true, Online: true, Level: "good", Path: "anfitrião", profile: profile{"m", "policeofficer"}},
		{Name: "rafa", Online: true, PingMs: 38, Level: "good", Path: "direta", profile: profile{"f", "doctor"}},
		{Name: "duda", Online: true, PingMs: 96, Level: "ok", Path: "direta", profile: profile{"f", "chef"}},
		{Name: "ze-do-mato", Online: true, PingMs: 182, Level: "bad", Path: "via relay sao", profile: profile{"m", "lumberjack"}},
		{Name: "lia", Online: true, PingMs: 54, Level: "good", Path: "direta", profile: profile{"f", "fireofficer"}},
		{Name: "beto", Level: "wait", Path: "offline", profile: profile{"m", "mechanics"}},
	}
	for i := 0; i < n && i < len(fake); i++ {
		p := lobbyPlayer{rosterEntry: fake[i], Seed: fmt.Sprint(seedFor(fake[i].Name))}
		a.decorate(&p, st.GameFound)
		st.Players = append(st.Players, p)
	}
	return st
}
