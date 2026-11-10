package main

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"
)

// TestChain sends packets game-client → joinForward → hostForward → game server
// and back, using plain loopback UDP in place of the tailnet.
func TestChain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// fake game server: replies "eco:" + payload
	srv, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	go func() {
		b := make([]byte, 2048)
		for {
			n, a, err := srv.ReadFromUDP(b)
			if err != nil {
				return
			}
			srv.WriteToUDP(append([]byte("eco:"), b[:n]...), a)
		}
	}()
	gamePort := srv.LocalAddr().(*net.UDPAddr).Port

	// "tailnet" side of the host
	hostPC, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var trH, trJ traffic
	go hostForward(ctx, hostPC, gamePort, &trH)

	// friend side
	local, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	dial := func(ctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "udp", hostPC.LocalAddr().String())
	}
	go joinForward(ctx, local, 0, dial, &trJ)

	// two game clients must get their own replies
	for c := 0; c < 2; c++ {
		cl, err := net.DialUDP("udp", nil, local.LocalAddr().(*net.UDPAddr))
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 5; i++ {
			msg := fmt.Sprintf("c%d-%d", c, i)
			cl.Write([]byte(msg))
			cl.SetReadDeadline(time.Now().Add(2 * time.Second))
			b := make([]byte, 2048)
			n, err := cl.Read(b)
			if err != nil {
				t.Fatalf("cliente %d msg %d: %v", c, i, err)
			}
			if got := string(b[:n]); got != "eco:"+msg {
				t.Fatalf("esperava eco:%s, veio %s", msg, got)
			}
		}
		cl.Close()
	}
	if trJ.up.Load() == 0 || trH.down.Load() == 0 {
		t.Fatal("contadores de tráfego zerados")
	}
}

func TestInviteRoundTrip(t *testing.T) {
	in := invite{Key: "tskey-auth-abc", Host: "pzhost-1a2b", Ports: []int{16261, 16262}}
	code := encodeInvite(in)
	out, err := decodeInvite("  " + code[:10] + "\n" + code[10:] + " ")
	if err != nil {
		t.Fatal(err)
	}
	if out.Key != in.Key || out.Host != in.Host || len(out.Ports) != 2 {
		t.Fatalf("convite mudou: %+v", out)
	}
	if _, err := decodeInvite("PZL1-!!!"); err == nil {
		t.Fatal("convite corrompido deveria falhar")
	}
}

func TestPingSummary(t *testing.T) {
	var p pingStats
	for i := 0; i < 20; i++ {
		p.add(pingSample{ok: true, ms: 40 + float64(i%2)*4, direct: true})
	}
	if s := p.summary(); s.Level != "good" || s.Path != "direta" {
		t.Fatalf("esperava ótima/direta: %+v", s)
	}
	for i := 0; i < 5; i++ {
		p.add(pingSample{ok: false})
	}
	if s := p.summary(); s.Level != "bad" || s.LossPct < 19 {
		t.Fatalf("esperava instável com perda: %+v", s)
	}
}
