package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Test data uses only fake values: the key is assembled at run time so secret
// scanners do not mistake it for a real one, and every address comes from the
// ranges reserved for documentation (RFC 5737 and RFC 3849).
func TestRedact(t *testing.T) {
	fakeKey := "tskey-" + "auth-" + "EXEMPLOfalso"
	in := "key " + fakeKey + " invite PZL1-EXEMPLOconvite ip 203.0.113.7:59117 " +
		"v6 [2001:db8:5b:8000::1]:41641 ts 100.100.1.2:33070 lan 10.0.0.5 lo 127.0.0.1:16261 hora 15:04:13"
	out := redact(in)
	for _, leak := range []string{"EXEMPLOfalso", "EXEMPLOconvite", "203.0.113.7", "2001:db8"} {
		if strings.Contains(out, leak) {
			t.Errorf("vazou %q: %s", leak, out)
		}
	}
	for _, keep := range []string{"100.100.1.2", "10.0.0.5", "127.0.0.1:16261", "15:04:13"} {
		if !strings.Contains(out, keep) {
			t.Errorf("apagou %q sem necessidade: %s", keep, out)
		}
	}
}

func TestInvitedHostPinsNodeKey(t *testing.T) {
	inv := invite{Host: "pzhost-ab12", Node: "nodekey:aaa"}
	if !isInvitedHost(inv, "pzhost-ab12", "nodekey:aaa") {
		t.Fatal("anfitrião verdadeiro recusado")
	}
	if isInvitedHost(inv, "pzhost-ab12", "nodekey:impostor") {
		t.Fatal("impostor com o mesmo nome foi aceito")
	}
	if isInvitedHost(inv, "pzhost-zz99", "nodekey:aaa") {
		t.Fatal("nome diferente foi aceito")
	}
	old := invite{Host: "pzhost-ab12"} // convite de versão antiga, sem chave do aparelho
	if !isInvitedHost(old, "pzhost-ab12", "nodekey:qualquer") {
		t.Fatal("convite antigo deveria continuar funcionando")
	}
}

func rawInvite(v any) string {
	b, _ := json.Marshal(v)
	return invitePrefix + base64.RawURLEncoding.EncodeToString(b)
}

func TestInviteRejectsHostileValues(t *testing.T) {
	bad := []map[string]any{
		{"k": "tskey-auth-x", "h": "pzhost-1", "p": []int{22}},                 // porta privilegiada
		{"k": "tskey-auth-x", "h": "pzhost-1", "p": []int{70000}},              // porta inválida
		{"k": "tskey-auth-x", "h": "outro-host", "p": []int{16261}},            // não é um anfitrião PZLink
		{"k": "nao-e-chave", "h": "pzhost-1", "p": []int{16261}},               // chave inválida
		{"k": "tskey-auth-x", "h": "pzhost-1", "n": "lixo", "p": []int{16261}}, // chave do aparelho inválida
		{"k": "tskey-auth-x", "h": "pzhost-1", "p": []int{2000, 2001, 2002, 2003, 2004, 2005, 2006, 2007, 2008}},
	}
	for i, v := range bad {
		if _, err := decodeInvite(rawInvite(v)); err == nil {
			t.Errorf("caso %d deveria ser recusado: %v", i, v)
		}
	}
	ok := map[string]any{"k": "tskey-auth-x", "h": "pzhost-1", "n": "nodekey:abc", "p": []int{16261, 16262}}
	if _, err := decodeInvite(rawInvite(ok)); err != nil {
		t.Fatalf("convite válido recusado: %v", err)
	}
}

// A peer that keeps opening new source ports must not grow the session table
// without bound.
func TestSessionLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	defer srv.Close()
	pc, _ := net.ListenPacket("udp", "127.0.0.1:0")
	var dials atomic.Int64
	var tr traffic
	done := make(chan struct{})
	go func() {
		forward(ctx, 0, &tr, "teste",
			func(b []byte) (int, net.Addr, error) { return pc.ReadFrom(b) },
			func(b []byte, to net.Addr) { pc.WriteTo(b, to) },
			func(context.Context) (net.Conn, error) {
				dials.Add(1)
				return net.DialUDP("udp", nil, srv.LocalAddr().(*net.UDPAddr))
			},
			func() { pc.Close() },
		)
		close(done)
	}()

	var clients []*net.UDPConn
	for i := 0; i < maxSessionsPerPeer+30; i++ {
		c, err := net.DialUDP("udp", nil, pc.LocalAddr().(*net.UDPAddr))
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, c)
		c.Write([]byte("x"))
	}
	deadline := time.Now().Add(3 * time.Second)
	for dials.Load() < maxSessionsPerPeer+30 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if dials.Load() != maxSessionsPerPeer+30 {
		t.Fatalf("esperava %d sessões criadas, foram %d", maxSessionsPerPeer+30, dials.Load())
	}
	for _, c := range clients {
		c.Close()
	}
	cancel()
	<-done
}

func TestSessTableEvictsLRU(t *testing.T) {
	tb := newSessTable()
	tb.mu.Lock()
	for i := 0; i < maxSessionsPerPeer+5; i++ {
		tb.makeRoom("100.100.9.9")
		e := &sessEntry{conn: nopCloser{}, peer: "100.100.9.9"}
		e.last.Store(int64(i + 1))
		tb.m[fmt.Sprintf("100.100.9.9:%d", i)] = e
	}
	n := len(tb.m)
	_, oldestKept := tb.m["100.100.9.9:0"]
	_, newestKept := tb.m[fmt.Sprintf("100.100.9.9:%d", maxSessionsPerPeer+4)]
	tb.mu.Unlock()
	if n != maxSessionsPerPeer {
		t.Fatalf("esperava %d sessões, ficaram %d", maxSessionsPerPeer, n)
	}
	if oldestKept || !newestKept {
		t.Fatal("deveria descartar a sessão mais antiga e manter a mais nova")
	}
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

func TestProbeDoesNotBindPort(t *testing.T) {
	// porta livre: nada escutando
	l, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	free := l.LocalAddr().(*net.UDPAddr).Port
	l.Close()
	if udpListening(free) {
		t.Fatal("porta livre detectada como servidor rodando")
	}
	// porta ocupada: "servidor" escutando
	srv, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	defer srv.Close()
	port := srv.LocalAddr().(*net.UDPAddr).Port
	if !udpListening(port) {
		t.Fatal("servidor escutando não foi detectado")
	}
	// a checagem não pode ocupar a porta: o "servidor" continua recebendo
	c, _ := net.DialUDP("udp", nil, srv.LocalAddr().(*net.UDPAddr))
	defer c.Close()
	c.Write([]byte("oi"))
	srv.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := srv.ReadFromUDP(make([]byte, 16)); err != nil {
		t.Fatalf("servidor parou de receber depois da checagem: %v", err)
	}
}
