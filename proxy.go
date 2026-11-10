package main

import (
	"context"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	sessionIdle = 90 * time.Second
	// Limits keep a misbehaving or malicious peer from exhausting sockets on
	// the other side. The game opens many short-lived query sockets, so when a
	// limit is hit the least recently used session is dropped instead of
	// refusing the new one.
	maxSessionsPerPeer = 48
	maxSessionsTotal   = 512
)

// traffic counts forwarded bytes for the UI.
type traffic struct {
	up, down atomic.Int64
}

type sessEntry struct {
	conn io.Closer
	peer string
	last atomic.Int64
}

func (e *sessEntry) touch() { e.last.Store(time.Now().UnixNano()) }

// sessTable is a map of sessions with LRU eviction. Callers hold mu.
type sessTable struct {
	mu sync.Mutex
	m  map[string]*sessEntry
}

func newSessTable() *sessTable { return &sessTable{m: map[string]*sessEntry{}} }

func (t *sessTable) evict(key string) {
	if e := t.m[key]; e != nil {
		e.conn.Close()
		delete(t.m, key)
	}
}

// makeRoom evicts least recently used sessions so one more fits for peer.
func (t *sessTable) makeRoom(peer string) {
	for {
		n, oldestKey, oldestPeerKey := 0, "", ""
		var oldest, oldestPeer int64
		for k, e := range t.m {
			l := e.last.Load()
			if oldestKey == "" || l < oldest {
				oldestKey, oldest = k, l
			}
			if e.peer == peer {
				n++
				if oldestPeerKey == "" || l < oldestPeer {
					oldestPeerKey, oldestPeer = k, l
				}
			}
		}
		switch {
		case n >= maxSessionsPerPeer:
			t.evict(oldestPeerKey)
		case len(t.m) >= maxSessionsTotal:
			t.evict(oldestKey)
		default:
			return
		}
	}
}

func (t *sessTable) closeAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for k := range t.m {
		t.evict(k)
	}
}

func (t *sessTable) reapLoop(ctx context.Context) {
	tk := time.NewTicker(15 * time.Second)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			now := time.Now().UnixNano()
			t.mu.Lock()
			for k, e := range t.m {
				if time.Duration(now-e.last.Load()) > sessionIdle {
					t.evict(k)
				}
			}
			t.mu.Unlock()
		}
	}
}

func (t *sessTable) len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.m)
}

func hostOf(a net.Addr) string {
	if h, _, err := net.SplitHostPort(a.String()); err == nil {
		return h
	}
	return a.String()
}

// hostForward receives game traffic arriving over the tailnet on pc and relays
// each remote peer to the local game server at 127.0.0.1:port.
func hostForward(ctx context.Context, pc net.PacketConn, port int, tr *traffic) {
	forward(ctx, port, tr, "host",
		func(b []byte) (int, net.Addr, error) { return pc.ReadFrom(b) },
		func(b []byte, to net.Addr) { pc.WriteTo(b, to) },
		func(context.Context) (net.Conn, error) {
			return net.DialUDP("udp", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		},
		func() { pc.Close() },
	)
}

// joinForward listens on 127.0.0.1:port for the local game client and relays
// each client socket to the host over the tailnet using dial.
func joinForward(ctx context.Context, local *net.UDPConn, port int, dial func(ctx context.Context) (net.Conn, error), tr *traffic) {
	forward(ctx, port, tr, "join",
		func(b []byte) (int, net.Addr, error) { return local.ReadFrom(b) },
		func(b []byte, to net.Addr) { local.WriteTo(b, to) },
		dial,
		func() { local.Close() },
	)
}

// forward is the shared UDP relay: every source address read from the
// listening side gets its own upstream connection, and replies on that
// connection are written back to the source.
func forward(ctx context.Context, port int, tr *traffic, side string,
	read func([]byte) (int, net.Addr, error),
	reply func([]byte, net.Addr),
	dial func(context.Context) (net.Conn, error),
	closeListener func(),
) {
	sessions := newSessTable()
	go func() {
		<-ctx.Done()
		closeListener()
		sessions.closeAll()
	}()
	go sessions.reapLoop(ctx)

	buf := make([]byte, 65535)
	for {
		n, src, err := read(buf)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("%s :%d read: %v", side, port, err)
			}
			return
		}
		key := src.String()
		sessions.mu.Lock()
		e := sessions.m[key]
		if e == nil {
			sessions.makeRoom(hostOf(src))
			dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			c, err := dial(dctx)
			cancel()
			if err != nil {
				sessions.mu.Unlock()
				log.Printf("%s :%d dial: %v", side, port, err)
				continue
			}
			e = &sessEntry{conn: c, peer: hostOf(src)}
			e.touch()
			sessions.m[key] = e
			go func(c net.Conn, e *sessEntry, src net.Addr) {
				b := make([]byte, 65535)
				for {
					n, err := c.Read(b)
					if err != nil {
						return
					}
					e.touch()
					tr.down.Add(int64(n))
					reply(b[:n], src)
				}
			}(c, e, src)
			log.Printf("%s :%d nova sessão de %s", side, port, key)
		}
		sessions.mu.Unlock()
		e.touch()
		tr.up.Add(int64(n))
		e.conn.(net.Conn).Write(buf[:n])
	}
}
