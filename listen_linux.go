package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// udpListening reads /proc/net/udp{,6} (what ss/netstat show).
func udpListening(port int) bool {
	want := fmt.Sprintf(":%04X", port)
	for _, f := range []string{"/proc/net/udp", "/proc/net/udp6"} {
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(fh)
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) > 1 && strings.HasSuffix(fields[1], want) {
				fh.Close()
				return true
			}
		}
		fh.Close()
	}
	return false
}
