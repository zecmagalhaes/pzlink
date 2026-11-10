package main

import (
	"encoding/binary"
	"syscall"
	"unsafe"
)

var procGetExtendedUdpTable = syscall.NewLazyDLL("iphlpapi.dll").NewProc("GetExtendedUdpTable")

const (
	afInet        = 2
	afInet6       = 23
	udpTableBasic = 0
	errInsuffBuf  = 122
)

// udpListening reads the IPv4 and IPv6 UDP tables (what netstat shows).
func udpListening(port int) bool {
	// MIB_UDPROW: localAddr(4) localPort(4); MIB_UDP6ROW: addr(16) scope(4) port(4)
	return udpTableHas(afInet, 8, 4, port) || udpTableHas(afInet6, 24, 20, port)
}

func udpTableHas(af uint32, rowSize, portOff, port int) bool {
	size := uint32(16 * 1024)
	for tries := 0; tries < 4; tries++ {
		buf := make([]byte, size)
		r, _, _ := procGetExtendedUdpTable.Call(
			uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)),
			0, uintptr(af), udpTableBasic, 0)
		if r == errInsuffBuf {
			continue // size was updated with the required length
		}
		if r != 0 {
			return false
		}
		n := int(binary.LittleEndian.Uint32(buf[0:4]))
		for i := 0; i < n; i++ {
			off := 4 + i*rowSize + portOff
			if off+2 > len(buf) {
				break
			}
			// the port is stored in network byte order in the low 16 bits
			if int(binary.BigEndian.Uint16(buf[off:off+2])) == port {
				return true
			}
		}
		return false
	}
	return false
}
