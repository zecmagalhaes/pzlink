package main

import (
	"encoding/binary"
	"errors"
	"net"
	"os"
	"sync"
	"time"
)

// gameProbe reports whether the game server is listening on a local UDP port.
// It must never bind that port, not even briefly: if the game started at that
// moment it would fail with "address in use". udpListening (per OS) reads the
// system's socket table instead.
type gameProbe struct {
	mu      sync.Mutex
	checked time.Time
	last    bool
}

const probeEvery = 3 * time.Second

func (g *gameProbe) running(port int) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if time.Since(g.checked) < probeEvery {
		return g.last
	}
	g.checked = time.Now()
	g.last = udpListening(port)
	return g.last
}

var raknetMagic = []byte{0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe, 0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78}

// probeUDP is the fallback for systems without a readable socket table. It
// sends a RakNet unconnected ping to 127.0.0.1:port; on Unix an ICMP
// port-unreachable shows up as a read error when nothing listens.
func probeUDP(port int, wait time.Duration) bool {
	c, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		return false
	}
	defer c.Close()
	ping := make([]byte, 0, 33)
	ping = append(ping, 0x01) // ID_UNCONNECTED_PING
	ping = binary.BigEndian.AppendUint64(ping, uint64(time.Now().UnixMilli()))
	ping = append(ping, raknetMagic...)
	ping = binary.BigEndian.AppendUint64(ping, 0)
	if _, err := c.Write(ping); err != nil {
		return false
	}
	c.SetReadDeadline(time.Now().Add(wait))
	buf := make([]byte, 1500)
	_, err = c.Read(buf)
	return err == nil || errors.Is(err, os.ErrDeadlineExceeded)
}
