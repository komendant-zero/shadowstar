package tun

import (
	"io"
	"log"
	"net"
	"sync/atomic"

	"github.com/shadowstar/shadowstar/pkg/client"
	"github.com/shadowstar/shadowstar/pkg/dns"
	"github.com/shadowstar/shadowstar/pkg/routing"
)

// PacketRelay acts as a virtual TUN device bridged over a local UDP socket.
// It allows Android VpnService or other unprivileged frontends to pass raw L3 IP packets
// directly into Go's NetStack.
type PacketRelay struct {
	listenAddr string
	conn       *net.UDPConn
	clientAddr atomic.Pointer[net.UDPAddr]
	netstack   *NetStack
	running    atomic.Bool
	stopChan   chan struct{}
}

// NewPacketRelay creates a UDP-based L3 packet relay listening on listenAddr (e.g. 127.0.0.1:10811)
func NewPacketRelay(listenAddr string, c *client.Client, r routing.Engine, f dns.FakeDNS) (*PacketRelay, error) {
	uAddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", uAddr)
	if err != nil {
		return nil, err
	}
	_ = conn.SetReadBuffer(2 * 1024 * 1024)
	_ = conn.SetWriteBuffer(2 * 1024 * 1024)

	relay := &PacketRelay{
		listenAddr: listenAddr,
		conn:       conn,
		stopChan:   make(chan struct{}),
	}

	if r == nil {
		r, _ = routing.NewEngine(routing.DefaultConfig())
	}
	if f == nil {
		f, _ = dns.NewFakeDNS("198.18.0.0/15")
	}

	ns := NewNetStack(relay, c, r, f)
	relay.netstack = ns

	return relay, nil
}

// Start launches the NetStack engine processing packets from the relay
func (pr *PacketRelay) Start() {
	if pr.running.Swap(true) {
		return
	}
	pr.netstack.Start()
	log.Printf("[PacketRelay] Android L3 Packet Relay active on %s", pr.listenAddr)
}

// Stop cleanly terminates the packet relay and netstack
func (pr *PacketRelay) Stop() error {
	if !pr.running.Swap(false) {
		return nil
	}
	close(pr.stopChan)
	pr.netstack.Stop()
	return pr.conn.Close()
}

// SetGameMode configures game acceleration and unblocking in the underlying NetStack
func (pr *PacketRelay) SetGameMode(enabled bool) {
	if pr.netstack != nil {
		pr.netstack.SetGameMode(enabled)
	}
}

// IsGameMode returns whether game mode is active in NetStack
func (pr *PacketRelay) IsGameMode() bool {
	if pr.netstack != nil {
		return pr.netstack.IsGameMode()
	}
	return false
}

// Read implements io.Reader: reads raw IP packet sent by Android VpnService over UDP
func (pr *PacketRelay) Read(p []byte) (int, error) {
	for pr.running.Load() {
		n, rAddr, err := pr.conn.ReadFromUDP(p)
		if err != nil {
			if !pr.running.Load() {
				return 0, io.EOF
			}
			return 0, err
		}
		if n > 0 {
			curr := pr.clientAddr.Load()
			if curr == nil || curr.Port != rAddr.Port || !curr.IP.Equal(rAddr.IP) {
				pr.clientAddr.Store(rAddr)
			}
			return n, nil
		}
	}
	return 0, io.EOF
}

// Write implements io.Writer: sends processed raw IP packet back to Android VpnService
func (pr *PacketRelay) Write(p []byte) (int, error) {
	target := pr.clientAddr.Load()
	if target == nil {
		// No Android VpnService packet received yet, drop return packet
		return len(p), nil
	}

	return pr.conn.WriteToUDP(p, target)
}

// Name implements Device
func (pr *PacketRelay) Name() string {
	return "android-packet-relay"
}

// Close implements io.Closer
func (pr *PacketRelay) Close() error {
	return pr.Stop()
}
