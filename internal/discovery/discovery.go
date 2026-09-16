package discovery

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// LAN discovery via UDP broadcast (no mDNS dependency).
// Daemons beacon every 5s on UDP :53318; Browse listens for 3s.
// Works on same-subnet WiFi without extra permissions; manual peers remain as fallback.

const beaconPort = 53318

type beacon struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Port int    `json:"port"`
}

// Peer is one discovered device.
type Peer struct {
	Name string
	ID   string
	Host string // ip
	Port int
	Addr string // host:port
}

// Advertise broadcasts this device until ctx cancelled. Call in a goroutine.
func Advertise(ctx interface {
	Done() <-chan struct{}
}, deviceName, deviceID string, port int) error {
	msg, _ := json.Marshal(beacon{ID: deviceID, Name: deviceName, Port: port})
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("255.255.255.255:%d", beaconPort))
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	// send immediately, then every tick
	_, _ = conn.Write(msg)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_, _ = conn.Write(msg)
		}
	}
}

// Browse listens for beacons for timeout and returns unique peers.
func Browse(timeout time.Duration) ([]Peer, error) {
	pc, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", beaconPort))
	if err != nil {
		return nil, err
	}
	defer pc.Close()
	_ = pc.SetDeadline(time.Now().Add(timeout))
	seen := map[string]Peer{}
	buf := make([]byte, 2048)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			break // timeout
		}
		var b beacon
		if err := json.Unmarshal(buf[:n], &b); err != nil {
			continue
		}
		host, _, _ := net.SplitHostPort(addr.String())
		if host == "" {
			continue
		}
		peerAddr := fmt.Sprintf("%s:%d", host, b.Port)
		key := b.ID
		if key == "" {
			key = peerAddr
		}
		if _, ok := seen[key]; !ok {
			seen[key] = Peer{Name: b.Name, ID: b.ID, Host: host, Port: b.Port, Addr: peerAddr}
		}
	}
	out := make([]Peer, 0, len(seen))
	for _, p := range seen {
		out = append(out, p)
	}
	return out, nil
}

// AddrsFromManual normalises ["192.168.1.5", "192.168.1.5:53317"] to host:port.
func AddrsFromManual(peers []string, defaultPort int) []string {
	out := []string{}
	for _, p := range peers {
		if p == "" {
			continue
		}
		if _, _, err := net.SplitHostPort(p); err != nil {
			out = append(out, fmt.Sprintf("%s:%d", p, defaultPort))
		} else {
			out = append(out, p)
		}
	}
	return out
}
