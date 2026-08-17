//go:build linux

package telemetry

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
)

const (
	sockDiagByFamily = 20
	inetDiagInfo     = 2
	nlmsgDone        = 3
	nlmsgError       = 2
	nlmFRequest      = 1
	nlmFRoot         = 0x100
	nlmFMatch        = 0x200
)

var ErrUnsupported = errors.New("TCP_INFO collection is supported only on Linux")

type SystemProvider struct{ allowNonRoot bool }

func NewSystemProvider(allowNonRoot bool) (TCPProvider, error) {
	if os.Geteuid() != 0 && !allowNonRoot {
		return nil, errors.New("tcpmetrics requires root/CAP_NET_ADMIN; set allow_non_root to attempt unprivileged collection")
	}
	return &SystemProvider{allowNonRoot: allowNonRoot}, nil
}

func (p *SystemProvider) Connections() ([]Connection, error) {
	var all []Connection
	for _, family := range []uint8{syscall.AF_INET, syscall.AF_INET6} {
		rows, err := dumpFamily(family)
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)
	}
	return all, nil
}

func dumpFamily(family uint8) ([]Connection, error) {
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_DGRAM, syscall.NETLINK_INET_DIAG)
	if err != nil {
		return nil, fmt.Errorf("open NETLINK_INET_DIAG: %w", err)
	}
	defer syscall.Close(fd)
	seq := uint32(1)
	req := make([]byte, 16+56)
	binary.LittleEndian.PutUint32(req[0:4], uint32(len(req)))
	binary.LittleEndian.PutUint16(req[4:6], sockDiagByFamily)
	binary.LittleEndian.PutUint16(req[6:8], nlmFRequest|nlmFRoot|nlmFMatch)
	binary.LittleEndian.PutUint32(req[8:12], seq)
	req[16], req[17], req[18] = family, syscall.IPPROTO_TCP, 1<<(inetDiagInfo-1)
	binary.LittleEndian.PutUint32(req[20:24], 0xffffffff)
	if err := syscall.Sendto(fd, req, 0, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}); err != nil {
		return nil, fmt.Errorf("request socket diagnostics: %w", err)
	}
	var out []Connection
	buf := make([]byte, 1<<20)
	for {
		n, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			return nil, fmt.Errorf("receive socket diagnostics: %w", err)
		}
		msgs, err := syscall.ParseNetlinkMessage(buf[:n])
		if err != nil {
			return nil, fmt.Errorf("parse socket diagnostics: %w", err)
		}
		for _, msg := range msgs {
			if msg.Header.Seq != seq {
				continue
			}
			switch msg.Header.Type {
			case nlmsgDone:
				return out, nil
			case nlmsgError:
				if len(msg.Data) >= 4 {
					errno := int32(binary.LittleEndian.Uint32(msg.Data[:4]))
					if errno != 0 {
						return nil, syscall.Errno(-errno)
					}
				}
			default:
				if row, ok := parseDiag(family, msg.Data); ok {
					out = append(out, row)
				}
			}
		}
	}
}

func parseDiag(family uint8, data []byte) (Connection, bool) {
	if len(data) < 72 {
		return Connection{}, false
	}
	local, remote := parseAddress(family, data[8:24]), parseAddress(family, data[24:40])
	if local == nil || remote == nil {
		return Connection{}, false
	}
	row := Connection{
		LocalAddress: local.String(), RemoteAddress: remote.String(),
		LocalPort: binary.BigEndian.Uint16(data[4:6]), RemotePort: binary.BigEndian.Uint16(data[6:8]),
		State: tcpState(data[1]), Inode: binary.LittleEndian.Uint32(data[68:72]),
	}
	row.ID = fmt.Sprintf("%s:%d-%s:%d-%d", row.LocalAddress, row.LocalPort, row.RemoteAddress, row.RemotePort, row.Inode)
	for attrs := data[72:]; len(attrs) >= 4; {
		length, typ := int(binary.LittleEndian.Uint16(attrs[:2])), binary.LittleEndian.Uint16(attrs[2:4])
		if length < 4 || length > len(attrs) {
			break
		}
		if typ == inetDiagInfo {
			parseTCPInfo(attrs[4:length], &row)
		}
		aligned := (length + 3) &^ 3
		if aligned > len(attrs) {
			break
		}
		attrs = attrs[aligned:]
	}
	return row, true
}

func parseAddress(family uint8, raw []byte) net.IP {
	if family == syscall.AF_INET {
		return net.IPv4(raw[0], raw[1], raw[2], raw[3])
	}
	if family == syscall.AF_INET6 {
		return net.IP(append([]byte(nil), raw[:16]...))
	}
	return nil
}

func parseTCPInfo(info []byte, row *Connection) {
	if len(info) >= 72 {
		row.RTT = binary.LittleEndian.Uint32(info[68:72])
	}
	if len(info) >= 84 {
		row.CongestionWnd = binary.LittleEndian.Uint32(info[80:84])
	}
	if len(info) >= 104 {
		row.Retransmits = uint64(binary.LittleEndian.Uint32(info[100:104]))
	}
	if len(info) >= 140 {
		row.SentSegments = uint64(binary.LittleEndian.Uint32(info[136:140]))
	}
}

func tcpState(state byte) string {
	names := [...]string{"UNKNOWN", "ESTABLISHED", "SYN_SENT", "SYN_RECV", "FIN_WAIT1", "FIN_WAIT2", "TIME_WAIT", "CLOSE", "CLOSE_WAIT", "LAST_ACK", "LISTEN", "CLOSING", "NEW_SYN_RECV"}
	if int(state) < len(names) {
		return names[state]
	}
	return fmt.Sprintf("STATE_%d", state)
}
