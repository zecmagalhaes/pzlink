//go:build !windows && !linux

package main

import "time"

func udpListening(port int) bool { return probeUDP(port, 400*time.Millisecond) }
