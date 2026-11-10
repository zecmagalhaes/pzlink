package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/tsnet"
)

var defaultPorts = []int{16261, 16262}

const (
	peerPrefix = "pzlink-"
	hostPrefix = "pzhost-"
)

// savedConfig is stored in the user's config dir. Invites are not saved: they
// carry an auth key and are only valid for one hosting session anyway.
type savedConfig struct {
	AuthKey  string `json:"authKey,omitempty"`
	GuestKey string `json:"guestKey,omitempty"`
	Nick     string `json:"nick,omitempty"`
}

type peerInfo struct {
	Name   string      `json:"name"`
	IP     string      `json:"ip"`
	Online bool        `json:"online"`
	Ping   pingSummary `json:"ping"`
}

type app struct {
	cfgDir string

	mu      sync.Mutex
	cfg     savedConfig
	mode    string // "", "host", "join"
	phase   string // texto curto para a UI
	errMsg  string
	invite  string
	selfIP  string
	hostIP  string
	ports   []int
	cancel  context.CancelFunc
	srv     *tsnet.Server
	stateD  string
	tr      traffic
	pings   map[string]*pingStats // por IP
	peers   []peerInfo
	others  []string // aparelhos da conta Tailscale que não são do PZLink
	started time.Time

	separateGuestKey bool
	game             gameProbe
}

func newApp(cfgDir string) *app {
	a := &app{cfgDir: cfgDir, pings: map[string]*pingStats{}}
	if b, err := os.ReadFile(filepath.Join(cfgDir, "config.json")); err == nil {
		json.Unmarshal(b, &a.cfg)
		// rewrite to drop fields from older versions (e.g. a saved invite)
		a.saveConfig()
	}
	return a
}

func (a *app) saveConfig() {
	b, _ := json.MarshalIndent(a.cfg, "", "  ")
	os.WriteFile(filepath.Join(a.cfgDir, "config.json"), b, 0o600)
}

func (a *app) setPhase(p string) {
	a.mu.Lock()
	a.phase = p
	a.mu.Unlock()
	log.Println("fase:", p)
}

func (a *app) fail(err error) {
	log.Println("erro:", err)
	a.mu.Lock()
	a.errMsg = err.Error()
	a.mu.Unlock()
	a.stop()
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// newServer creates an ephemeral tsnet node with a fresh state directory.
func (a *app) newServer(hostname, key string) (*tsnet.Server, string, error) {
	dir, err := os.MkdirTemp("", "pzlink-state-")
	if err != nil {
		return nil, "", err
	}
	logf := func(format string, args ...any) { log.Printf("[ts] "+format, args...) }
	s := &tsnet.Server{
		Dir:       dir,
		Hostname:  hostname,
		AuthKey:   key,
		Ephemeral: true,
		Logf:      logf,
		UserLogf:  logf,
	}
	return s, dir, nil
}

func (a *app) begin(mode string) (context.Context, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.mode != "" {
		return nil, errors.New("já está rodando; clique em Parar primeiro")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mode, a.cancel, a.errMsg, a.invite = mode, cancel, "", ""
	a.selfIP, a.hostIP, a.peers, a.others = "", "", nil, nil
	a.pings = map[string]*pingStats{}
	a.tr.up.Store(0)
	a.tr.down.Store(0)
	a.started = time.Now()
	return ctx, nil
}

func (a *app) up(ctx context.Context, s *tsnet.Server) (netip.Addr, error) {
	a.setPhase("conectando ao Tailscale...")
	uctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if _, err := s.Up(uctx); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "invalid key") || strings.Contains(err.Error(), "NeedsLogin") || uctx.Err() != nil {
			return netip.Addr{}, fmt.Errorf("não consegui entrar na rede Tailscale. A chave pode estar errada, vencida ou já ter sido apagada (%v)", err)
		}
		return netip.Addr{}, err
	}
	ip4, _ := s.TailscaleIPs()
	if !ip4.IsValid() {
		return netip.Addr{}, errors.New("o Tailscale não deu um IP para este PC")
	}
	return ip4, nil
}

// ---------- anfitrião ----------

func (a *app) startHost(key, guestKey string) error {
	key, guestKey = strings.TrimSpace(key), strings.TrimSpace(guestKey)
	if key == "" {
		key = a.cfg.AuthKey
	}
	if guestKey == "" {
		guestKey = a.cfg.GuestKey
	}
	if !strings.HasPrefix(key, "tskey-") {
		return errors.New("cole a chave do Tailscale (começa com tskey-auth-)")
	}
	if guestKey != "" && !strings.HasPrefix(guestKey, "tskey-") {
		return errors.New("a chave dos convidados também deve começar com tskey-auth-")
	}
	if guestKey == key {
		guestKey = ""
	}
	inviteKey := key
	if guestKey != "" {
		inviteKey = guestKey
	}
	ctx, err := a.begin("host")
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.ports = defaultPorts
	a.mu.Unlock()

	go func() {
		hostname := hostPrefix + randHex(2)
		s, dir, err := a.newServer(hostname, key)
		if err != nil {
			a.fail(err)
			return
		}
		a.mu.Lock()
		a.srv, a.stateD = s, dir
		a.mu.Unlock()

		ip, err := a.up(ctx, s)
		if err != nil {
			a.fail(err)
			return
		}
		lc, err := s.LocalClient()
		if err != nil {
			a.fail(err)
			return
		}
		st, err := lc.StatusWithoutPeers(ctx)
		if err != nil || st.Self == nil {
			a.fail(fmt.Errorf("não consegui ler a identidade deste PC no Tailscale: %v", err))
			return
		}
		nodeKey := st.Self.PublicKey.String()
		for _, p := range defaultPorts {
			pc, err := s.ListenPacket("udp", fmt.Sprintf("%s:%d", ip, p))
			if err != nil {
				a.fail(fmt.Errorf("não consegui abrir a porta %d na rede Tailscale: %v", p, err))
				return
			}
			go hostForward(ctx, pc, p, &a.tr)
		}
		inv := encodeInvite(invite{Key: inviteKey, Host: hostname, Node: nodeKey, Ports: defaultPorts})
		a.mu.Lock()
		a.cfg.AuthKey, a.cfg.GuestKey = key, guestKey
		a.saveConfig()
		a.separateGuestKey = guestKey != ""
		a.selfIP, a.invite = ip.String(), inv
		a.mu.Unlock()
		a.setPhase("hospedando")
		go a.hostPeerLoop(ctx, s)
	}()
	return nil
}

func (a *app) hostPeerLoop(ctx context.Context, s *tsnet.Server) {
	lc, err := s.LocalClient()
	if err != nil {
		a.fail(err)
		return
	}
	pinging := map[string]bool{}
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		st, err := lc.Status(ctx)
		if err == nil {
			var list []peerInfo
			var others []string
			for _, p := range st.Peer {
				if !strings.HasPrefix(p.HostName, peerPrefix) && !strings.HasPrefix(p.HostName, hostPrefix) {
					others = append(others, p.HostName)
					continue
				}
				if !strings.HasPrefix(p.HostName, peerPrefix) || len(p.TailscaleIPs) == 0 {
					continue
				}
				ip := p.TailscaleIPs[0]
				for _, x := range p.TailscaleIPs {
					if x.Is4() {
						ip = x
					}
				}
				ps := a.statsFor(ip.String())
				if p.Online && !pinging[ip.String()] {
					pinging[ip.String()] = true
					go a.pingLoop(ctx, lc, ip, ps, 2*time.Second)
				}
				list = append(list, peerInfo{
					Name:   strings.TrimPrefix(p.HostName, peerPrefix),
					IP:     ip.String(),
					Online: p.Online,
					Ping:   ps.summary(),
				})
			}
			sort.Slice(list, func(i, j int) bool {
				if list[i].Online != list[j].Online {
					return list[i].Online
				}
				return list[i].Name < list[j].Name
			})
			sort.Strings(others)
			a.mu.Lock()
			a.peers, a.others = list, others
			a.mu.Unlock()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ---------- amigo ----------

func (a *app) startJoin(code, nick string) error {
	inv, err := decodeInvite(code)
	if err != nil {
		return err
	}
	nick = sanitizeName(nick)
	if nick == "" {
		return errors.New("escolha um apelido (letras e números)")
	}
	// confere as portas antes de ligar o Tailscale
	var locals []*net.UDPConn
	for _, p := range inv.Ports {
		c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: p})
		if err != nil {
			for _, l := range locals {
				l.Close()
			}
			return fmt.Errorf("a porta %d já está em uso neste PC. Feche o servidor do jogo ou outro PZLink que esteja aberto", p)
		}
		locals = append(locals, c)
	}
	ctx, err := a.begin("join")
	if err != nil {
		for _, l := range locals {
			l.Close()
		}
		return err
	}
	a.mu.Lock()
	a.cfg.Nick = nick
	a.saveConfig()
	a.ports = inv.Ports
	a.mu.Unlock()

	go func() {
		<-ctx.Done()
		for _, l := range locals {
			l.Close()
		}
	}()

	go func() {
		s, dir, err := a.newServer(peerPrefix+nick+"-"+randHex(1), inv.Key)
		if err != nil {
			a.fail(err)
			return
		}
		a.mu.Lock()
		a.srv, a.stateD = s, dir
		a.mu.Unlock()

		ip, err := a.up(ctx, s)
		if err != nil {
			a.fail(err)
			return
		}
		a.mu.Lock()
		a.selfIP = ip.String()
		a.mu.Unlock()

		lc, err := s.LocalClient()
		if err != nil {
			a.fail(err)
			return
		}
		a.setPhase("procurando o anfitrião...")
		var hostIP netip.Addr
		deadline := time.Now().Add(40 * time.Second)
		for !hostIP.IsValid() {
			st, err := lc.Status(ctx)
			if err == nil {
				for _, p := range st.Peer {
					if !isInvitedHost(inv, p.HostName, p.PublicKey.String()) || !p.Online {
						continue
					}
					for _, x := range p.TailscaleIPs {
						if x.Is4() {
							hostIP = x
						}
					}
				}
			}
			if hostIP.IsValid() {
				break
			}
			if time.Now().After(deadline) {
				a.fail(errors.New("o anfitrião não está online. Peça para ele abrir o PZLink e clicar em Hospedar, ou mandar um convite novo"))
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
		a.mu.Lock()
		a.hostIP = hostIP.String()
		a.mu.Unlock()

		for i, p := range inv.Ports {
			target := fmt.Sprintf("%s:%d", hostIP, p)
			dial := func(dctx context.Context) (net.Conn, error) { return s.Dial(dctx, "udp", target) }
			go joinForward(ctx, locals[i], p, dial, &a.tr)
		}
		a.setPhase("conectado")
		go a.pingLoop(ctx, lc, hostIP, a.statsFor(hostIP.String()), time.Second)
	}()
	return nil
}

// isInvitedHost reports whether a tailnet peer is the host named in the invite.
// When the invite pins the host's node key, the hostname alone is not enough:
// anyone holding the invite's auth key could register a node with that name.
func isInvitedHost(inv invite, hostName, nodeKey string) bool {
	if hostName != inv.Host {
		return false
	}
	return inv.Node == "" || nodeKey == inv.Node
}

// ---------- comum ----------

func (a *app) statsFor(ip string) *pingStats {
	a.mu.Lock()
	defer a.mu.Unlock()
	ps := a.pings[ip]
	if ps == nil {
		ps = &pingStats{}
		a.pings[ip] = ps
	}
	return ps
}

type localClient interface {
	Ping(ctx context.Context, ip netip.Addr, pingtype tailcfg.PingType) (*ipnstate.PingResult, error)
}

func (a *app) pingLoop(ctx context.Context, lc localClient, ip netip.Addr, ps *pingStats, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		pctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		res, err := lc.Ping(pctx, ip, tailcfg.PingDisco)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err != nil || res == nil || res.Err != "" {
			ps.add(pingSample{ok: false})
		} else {
			ps.add(pingSample{
				ok:      true,
				ms:      res.LatencySeconds * 1000,
				direct:  res.Endpoint != "" && res.DERPRegionID == 0,
				derp:    res.DERPRegionCode,
				address: res.Endpoint,
			})
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (a *app) stop() {
	a.mu.Lock()
	cancel, srv, dir := a.cancel, a.srv, a.stateD
	a.mode, a.cancel, a.srv, a.stateD = "", nil, nil, ""
	a.phase, a.invite, a.peers, a.others, a.hostIP, a.selfIP = "", "", nil, nil, "", ""
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if srv != nil {
		srv.Close()
	}
	if dir != "" {
		os.RemoveAll(dir)
	}
}

type uiState struct {
	Version     string       `json:"version"`
	Mode        string       `json:"mode"`
	Phase       string       `json:"phase"`
	Error       string       `json:"error"`
	Invite      string       `json:"invite"`
	SelfIP      string       `json:"selfIP"`
	HostIP      string       `json:"hostIP"`
	Ports       []int        `json:"ports"`
	GameRunning bool         `json:"gameRunning"`
	HasKey      bool         `json:"hasKey"`
	HasGuestKey bool         `json:"hasGuestKey"`
	GuestKeyOn  bool         `json:"guestKeyOn"`
	Nick        string       `json:"nick"`
	Peers       []peerInfo   `json:"peers"`
	Others      []string     `json:"others"`
	HostPing    *pingSummary `json:"hostPing,omitempty"`
	UpBytes     int64        `json:"upBytes"`
	DownBytes   int64        `json:"downBytes"`
	Uptime      int64        `json:"uptime"`
}

func (a *app) snapshot() uiState {
	a.mu.Lock()
	st := uiState{
		Version: version, Mode: a.mode, Phase: a.phase, Error: a.errMsg,
		Invite: a.invite, SelfIP: a.selfIP, HostIP: a.hostIP, Ports: a.ports,
		HasKey: a.cfg.AuthKey != "", HasGuestKey: a.cfg.GuestKey != "",
		GuestKeyOn: a.mode == "host" && a.separateGuestKey,
		Nick:       a.cfg.Nick, Peers: a.peers, Others: a.others,
	}
	hostPS := a.pings[a.hostIP]
	if a.mode != "" {
		st.Uptime = int64(time.Since(a.started).Seconds())
	}
	a.mu.Unlock()
	if st.Mode == "host" {
		st.GameRunning = a.game.running(defaultPorts[0])
	}
	if hostPS != nil {
		s := hostPS.summary()
		st.HostPing = &s
	}
	st.UpBytes, st.DownBytes = a.tr.up.Load(), a.tr.down.Load()
	if st.Peers == nil {
		st.Peers = []peerInfo{}
	}
	return st
}
