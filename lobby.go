package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/zecmagalhaes/pzlink/pzasset"
	"tailscale.com/tsnet"
)

// The host publishes the lobby roster inside the tailnet; guests fetch it.
// Only names and pings travel: every PZLink builds the 3D characters from its
// own game installation, seeded by the player's nickname.
const rosterPort = 7471

type rosterEntry struct {
	Name   string  `json:"name"`
	Host   bool    `json:"host,omitempty"`
	PingMs float64 `json:"pingMs"`
	Level  string  `json:"level"`
	Path   string  `json:"path,omitempty"`
}

type lobbyPlayer struct {
	rosterEntry
	Self      bool               `json:"self,omitempty"`
	Seed      string             `json:"seed"`
	Character *pzasset.Character `json:"character,omitempty"`
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

	idleSpeed float64

	mu  sync.Mutex
	glb map[uint64][]byte
}

func (as *assets) load() error {
	as.once.Do(func() {
		as.game, as.err = pzasset.FindGame()
		if as.err == nil {
			as.cat, as.err = as.game.LoadCatalog()
			as.idleSpeed = as.game.IdleSpeed()
		}
		if as.err != nil {
			log.Println("lobby:", as.err)
		}
		as.glb = map[uint64][]byte{}
	})
	return as.err
}

func (as *assets) characterGLB(seed uint64) ([]byte, error) {
	if err := as.load(); err != nil {
		return nil, err
	}
	as.mu.Lock()
	if b, ok := as.glb[seed]; ok {
		as.mu.Unlock()
		return b, nil
	}
	as.mu.Unlock()
	ch, err := as.cat.NewCharacter(seed)
	if err != nil {
		return nil, err
	}
	b, err := as.cat.GLB(ch)
	if err != nil {
		return nil, err
	}
	as.mu.Lock()
	if len(as.glb) >= 32 {
		as.glb = map[uint64][]byte{}
	}
	as.glb[seed] = b
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
	return "voce"
}

// hostRoster is the roster as the host sees it.
func (a *app) hostRoster() []rosterEntry {
	out := []rosterEntry{{Name: a.selfNick(), Host: true, Level: "good", Path: "anfitrião"}}
	a.mu.Lock()
	peers := append([]peerInfo(nil), a.peers...)
	a.mu.Unlock()
	for _, p := range peers {
		if !p.Online {
			continue
		}
		out = append(out, rosterEntry{Name: displayName(p.Name), PingMs: p.Ping.AvgMs, Level: p.Ping.Level, Path: p.Ping.Path})
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
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second,
		WriteTimeout: 5 * time.Second, MaxHeaderBytes: 8 << 10}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	srv.Serve(ln)
}

// fetchRosterLoop keeps a guest's copy of the host's roster.
func (a *app) fetchRosterLoop(ctx context.Context, s *tsnet.Server, hostIP string) {
	client := s.HTTPClient()
	client.Timeout = 4 * time.Second
	url := fmt.Sprintf("http://%s:%d/roster", hostIP, rosterPort)
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		if r, err := fetchRoster(ctx, client, url); err == nil {
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
	for _, e := range roster {
		p := lobbyPlayer{rosterEntry: e, Seed: fmt.Sprint(seedFor(e.Name))}
		p.Self = strings.EqualFold(e.Name, self) && (e.Host == (mode == "host"))
		if mode == "" {
			p.Self = true
		}
		if st.GameFound {
			p.Character, _ = a.assets.cat.NewCharacter(seedFor(e.Name))
		}
		st.Players = append(st.Players, p)
	}
	return st
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
		{Name: "anfitriao", Host: true, Level: "good", Path: "anfitrião"},
		{Name: "rafa", PingMs: 38, Level: "good", Path: "direta"},
		{Name: "duda", PingMs: 96, Level: "ok", Path: "direta"},
		{Name: "ze-do-mato", PingMs: 182, Level: "bad", Path: "via relay sao"},
		{Name: "lia", PingMs: 54, Level: "good", Path: "direta"},
		{Name: "beto", PingMs: 71, Level: "good", Path: "direta"},
	}
	for i := 0; i < n && i < len(fake); i++ {
		e := fake[i]
		p := lobbyPlayer{rosterEntry: e, Seed: fmt.Sprint(seedFor(e.Name))}
		if st.GameFound {
			p.Character, _ = a.assets.cat.NewCharacter(seedFor(e.Name))
		}
		st.Players = append(st.Players, p)
	}
	return st
}
