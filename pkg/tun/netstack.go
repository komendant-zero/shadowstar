package tun

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shadowstar/shadowstar/pkg/client"
	"github.com/shadowstar/shadowstar/pkg/dns"
	"github.com/shadowstar/shadowstar/pkg/routing"
)

// IP protocol numbers
const (
	ProtoICMP = 1
	ProtoTCP  = 6
	ProtoUDP  = 17
)

// TCP flags
const (
	TCPFlagFIN = 0x01
	TCPFlagSYN = 0x02
	TCPFlagRST = 0x04
	TCPFlagPSH = 0x08
	TCPFlagACK = 0x10
)

type tcpConnKey struct {
	srcIP   string
	dstIP   string
	srcPort uint16
	dstPort uint16
}

type activeTCPConn struct {
	key        tcpConnKey
	targetAddr string
	remoteConn io.ReadWriteCloser
	clientSeq  uint32
	serverSeq  uint32
	lastActive atomic.Int64
	closed     atomic.Bool
	writeMu    sync.Mutex
}

type udpConnKey struct {
	srcIP   string
	dstIP   string
	srcPort uint16
	dstPort uint16
}

type activeUDPConn struct {
	key        udpConnKey
	targetHost string
	targetPort uint16
	isProxy    bool
	directConn net.Conn
	socksTCP   net.Conn
	socksUDP   *net.UDPConn
	lastActive atomic.Int64
	closed     atomic.Bool
}

// NetStack processes raw IP packets arriving from TUN device
type NetStack struct {
	device      io.ReadWriter
	client      *client.Client
	router      routing.Engine
	fakeDNS     dns.FakeDNS
	dnsResolver *dns.Resolver
	socksAddr   string
	
	tcpConns map[tcpConnKey]*activeTCPConn
	tcpMu    sync.RWMutex
	udpConns map[udpConnKey]*activeUDPConn
	udpMu    sync.RWMutex
	running  atomic.Bool
	stopChan chan struct{}
}

// SetSocksAddr sets the upstream SOCKS5 proxy address (default: 127.0.0.1:10808)
func (ns *NetStack) SetSocksAddr(addr string) {
	ns.socksAddr = addr
}

func (ns *NetStack) getSocksAddr() string {
	if ns.socksAddr != "" {
		return ns.socksAddr
	}
	return "127.0.0.1:10808"
}

// NewNetStack creates a new L3 packet processor
func NewNetStack(dev io.ReadWriter, c *client.Client, r routing.Engine, f dns.FakeDNS) *NetStack {
	if r == nil {
		r, _ = routing.NewEngine(routing.DefaultConfig())
	}
	return &NetStack{
		device:   dev,
		client:   c,
		router:   r,
		fakeDNS:  f,
		tcpConns: make(map[tcpConnKey]*activeTCPConn),
		udpConns: make(map[udpConnKey]*activeUDPConn),
		stopChan: make(chan struct{}),
	}
}

// Start begins the packet processing loop
func (ns *NetStack) Start() {
	ns.running.Store(true)
	go ns.readLoop()
	go ns.reaperLoop()
}

// Stop cleanly terminates the netstack
func (ns *NetStack) Stop() {
	if !ns.running.Swap(false) {
		return
	}
	close(ns.stopChan)

	ns.tcpMu.Lock()
	for k, c := range ns.tcpConns {
		c.closed.Store(true)
		if c.remoteConn != nil {
			_ = c.remoteConn.Close()
		}
		delete(ns.tcpConns, k)
	}
	ns.tcpMu.Unlock()

	ns.udpMu.Lock()
	for k, c := range ns.udpConns {
		c.closed.Store(true)
		if c.directConn != nil {
			_ = c.directConn.Close()
		}
		if c.socksUDP != nil {
			_ = c.socksUDP.Close()
		}
		if c.socksTCP != nil {
			_ = c.socksTCP.Close()
		}
		delete(ns.udpConns, k)
	}
	ns.udpMu.Unlock()
}

func (ns *NetStack) readLoop() {
	buf := make([]byte, 65535)
	for ns.running.Load() {
		n, err := ns.device.Read(buf)
		if err != nil {
			if !ns.running.Load() {
				return
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if n < 20 {
			continue // Invalid IPv4 header
		}

		packet := make([]byte, n)
		copy(packet, buf[:n])
		go ns.processIPv4Packet(packet)
	}
}

func (ns *NetStack) processIPv4Packet(pkt []byte) {
	version := pkt[0] >> 4
	if version != 4 {
		return // IPv4 only for primary L3 stack
	}

	ihl := int(pkt[0]&0x0F) * 4
	if len(pkt) < ihl {
		return
	}

	protocol := pkt[9]
	srcIP := net.IP(pkt[12:16])
	dstIP := net.IP(pkt[16:20])
	payload := pkt[ihl:]

	switch protocol {
	case ProtoICMP:
		ns.handleICMP(srcIP, dstIP, payload)
	case ProtoUDP:
		ns.handleUDP(srcIP, dstIP, payload)
	case ProtoTCP:
		ns.handleTCP(srcIP, dstIP, payload)
	}
}

// handleICMP replies to ICMP Echo Requests (ping)
func (ns *NetStack) handleICMP(srcIP, dstIP net.IP, payload []byte) {
	if len(payload) < 8 || payload[0] != 8 { // Type 8: Echo Request
		return
	}

	// Build Echo Reply (Type 0)
	reply := make([]byte, len(payload))
	copy(reply, payload)
	reply[0] = 0 // Echo Reply
	reply[2] = 0 // Clear checksum
	reply[3] = 0
	cs := checksum(reply)
	binary.BigEndian.PutUint16(reply[2:4], cs)

	// Send IPv4 packet back: swap src and dst
	ns.sendIPv4Packet(dstIP, srcIP, ProtoICMP, reply)
}

// handleUDP intercepts DNS (port 53) and general UDP
func (ns *NetStack) handleUDP(srcIP, dstIP net.IP, payload []byte) {
	if len(payload) < 8 {
		return
	}

	srcPort := binary.BigEndian.Uint16(payload[0:2])
	dstPort := binary.BigEndian.Uint16(payload[2:4])
	udpLen := binary.BigEndian.Uint16(payload[4:6])
	if int(udpLen) > len(payload) || udpLen < 8 {
		return
	}
	udpData := payload[8:udpLen]

	// DNS queries (port 53)
	if dstPort == 53 {
		ns.handleDNSQuery(srcIP, dstIP, srcPort, udpData)
		return
	}

	key := udpConnKey{
		srcIP:   srcIP.String(),
		dstIP:   dstIP.String(),
		srcPort: srcPort,
		dstPort: dstPort,
	}

	// 1. Existing active UDP session
	ns.udpMu.RLock()
	connObj, exists := ns.udpConns[key]
	ns.udpMu.RUnlock()

	if exists && !connObj.closed.Load() {
		connObj.lastActive.Store(time.Now().Unix())
		if connObj.isProxy {
			pkt := buildSocksUDPPacket(connObj.targetHost, connObj.targetPort, udpData)
			_, _ = connObj.socksUDP.Write(pkt)
		} else {
			_, _ = connObj.directConn.Write(udpData)
		}
		return
	}

	// 2. New UDP session: evaluate route
	destHost := dstIP.String()
	if ns.fakeDNS != nil && ns.fakeDNS.IsFakeIP(dstIP) {
		if realDomain, found := ns.fakeDNS.LookupDomain(dstIP); found {
			destHost = realDomain
		}
	}
	destAddr := net.JoinHostPort(destHost, strconv.Itoa(int(dstPort)))

	action := routing.ActionProxy
	if ns.router != nil {
		action = ns.router.Match(destAddr)
	}

	if action == routing.ActionBlock {
		return
	}

	ns.udpMu.Lock()
	if connObj, exists := ns.udpConns[key]; exists && !connObj.closed.Load() {
		ns.udpMu.Unlock()
		connObj.lastActive.Store(time.Now().Unix())
		if connObj.isProxy {
			pkt := buildSocksUDPPacket(connObj.targetHost, connObj.targetPort, udpData)
			_, _ = connObj.socksUDP.Write(pkt)
		} else {
			_, _ = connObj.directConn.Write(udpData)
		}
		return
	}

	if action == routing.ActionDirect {
		dConn, err := net.DialTimeout("udp", destAddr, 3*time.Second)
		if err != nil {
			ns.udpMu.Unlock()
			return
		}
		c := &activeUDPConn{
			key:        key,
			targetHost: destHost,
			targetPort: dstPort,
			isProxy:    false,
			directConn: dConn,
		}
		c.lastActive.Store(time.Now().Unix())
		ns.udpConns[key] = c
		ns.udpMu.Unlock()

		_, _ = dConn.Write(udpData)
		go ns.forwardDirectUDPToTUN(c, srcIP, dstIP, srcPort, dstPort)
	} else {
		// ActionProxy: SOCKS5 UDP Associate via local SOCKS5 proxy
		tcpConn, err := net.DialTimeout("tcp", ns.getSocksAddr(), 4*time.Second)
		if err != nil {
			ns.udpMu.Unlock()
			return
		}
		relayAddr, err := socks5UDPAssociateHandshake(tcpConn)
		if err != nil {
			_ = tcpConn.Close()
			ns.udpMu.Unlock()
			return
		}
		uConn, err := net.DialUDP("udp", nil, relayAddr)
		if err != nil {
			_ = tcpConn.Close()
			ns.udpMu.Unlock()
			return
		}

		c := &activeUDPConn{
			key:        key,
			targetHost: destHost,
			targetPort: dstPort,
			isProxy:    true,
			socksTCP:   tcpConn,
			socksUDP:   uConn,
		}
		c.lastActive.Store(time.Now().Unix())
		ns.udpConns[key] = c
		ns.udpMu.Unlock()

		pkt := buildSocksUDPPacket(destHost, dstPort, udpData)
		_, _ = uConn.Write(pkt)
		go ns.forwardProxyUDPToTUN(c, srcIP, dstIP, srcPort, dstPort)
	}
}

func (ns *NetStack) forwardProxyUDPToTUN(connObj *activeUDPConn, srcIP, dstIP net.IP, srcPort, dstPort uint16) {
	defer func() {
		connObj.closed.Store(true)
		if connObj.socksUDP != nil {
			_ = connObj.socksUDP.Close()
		}
		if connObj.socksTCP != nil {
			_ = connObj.socksTCP.Close()
		}
		ns.udpMu.Lock()
		delete(ns.udpConns, connObj.key)
		ns.udpMu.Unlock()
	}()

	buf := make([]byte, 65535)
	for ns.running.Load() && !connObj.closed.Load() {
		_ = connObj.socksUDP.SetReadDeadline(time.Now().Add(60 * time.Second))
		n, err := connObj.socksUDP.Read(buf)
		if n > 0 {
			connObj.lastActive.Store(time.Now().Unix())
			payload := parseSocksUDPResponse(buf[:n])
			if len(payload) > 0 {
				ns.sendUDPPacket(dstIP, srcIP, dstPort, srcPort, payload)
			}
		}
		if err != nil {
			break
		}
	}
}

func (ns *NetStack) forwardDirectUDPToTUN(connObj *activeUDPConn, srcIP, dstIP net.IP, srcPort, dstPort uint16) {
	defer func() {
		connObj.closed.Store(true)
		if connObj.directConn != nil {
			_ = connObj.directConn.Close()
		}
		ns.udpMu.Lock()
		delete(ns.udpConns, connObj.key)
		ns.udpMu.Unlock()
	}()

	buf := make([]byte, 65535)
	for ns.running.Load() && !connObj.closed.Load() {
		_ = connObj.directConn.SetReadDeadline(time.Now().Add(60 * time.Second))
		n, err := connObj.directConn.Read(buf)
		if n > 0 {
			connObj.lastActive.Store(time.Now().Unix())
			ns.sendUDPPacket(dstIP, srcIP, dstPort, srcPort, buf[:n])
		}
		if err != nil {
			break
		}
	}
}

// handleDNSQuery resolves DNS query via FakeDNS or DoH
func (ns *NetStack) handleDNSQuery(clientIP, dnsServerIP net.IP, clientPort uint16, dnsData []byte) {
	if len(dnsData) < 12 {
		return
	}

	txID := binary.BigEndian.Uint16(dnsData[0:2])
	domain, qtype := parseDNSQuestion(dnsData[12:])
	if domain == "" {
		return
	}

	// 1. If A query (IPv4) and FakeDNS available, synthesize 198.18.0.x IP instantly
	if qtype == 1 && ns.fakeDNS != nil {
		fakeIP := ns.fakeDNS.Allocate(domain)
		if fakeIP != nil {
			resp := buildSyntheticDNSResponse(txID, dnsData, fakeIP.String())
			ns.sendUDPPacket(dnsServerIP, clientIP, 53, clientPort, resp)
			return
		}
	}

	// 2. Direct upstream resolution via client or default DoH
	go func() {
		ips, err := net.LookupIP(domain)
		if err == nil && len(ips) > 0 {
			var chosenIPv4 net.IP
			for _, ip := range ips {
				if v4 := ip.To4(); v4 != nil {
					chosenIPv4 = v4
					break
				}
			}
			if chosenIPv4 != nil {
				resp := buildSyntheticDNSResponse(txID, dnsData, chosenIPv4.String())
				ns.sendUDPPacket(dnsServerIP, clientIP, 53, clientPort, resp)
			}
		}
	}()
}

// handleTCP manages full TCP connection lifecycle (SYN -> DATA -> FIN/RST)
func (ns *NetStack) handleTCP(srcIP, dstIP net.IP, payload []byte) {
	if len(payload) < 20 {
		return
	}

	srcPort := binary.BigEndian.Uint16(payload[0:2])
	dstPort := binary.BigEndian.Uint16(payload[2:4])
	seq := binary.BigEndian.Uint32(payload[4:8])
	ack := binary.BigEndian.Uint32(payload[8:12])
	dataOffset := int(payload[12]>>4) * 4
	flags := payload[13]

	if len(payload) < dataOffset {
		return
	}
	tcpData := payload[dataOffset:]

	key := tcpConnKey{
		srcIP:   srcIP.String(),
		dstIP:   dstIP.String(),
		srcPort: srcPort,
		dstPort: dstPort,
	}

	// 1. Connection initiation: SYN
	if flags&TCPFlagSYN != 0 && flags&TCPFlagACK == 0 {
		destHost := dstIP.String()
		if ns.fakeDNS != nil && ns.fakeDNS.IsFakeIP(dstIP) {
			if dom, ok := ns.fakeDNS.LookupDomain(dstIP); ok {
				destHost = dom
			}
		}
		destAddr := net.JoinHostPort(destHost, strconv.Itoa(int(dstPort)))

		// Check routing decision
		action := routing.ActionProxy
		if ns.router != nil {
			action = ns.router.Match(destAddr)
		}

		if action == routing.ActionBlock {
			// Reply with RST
			ns.sendTCPPacket(dstIP, srcIP, dstPort, srcPort, ack, seq+1, TCPFlagRST|TCPFlagACK, nil)
			return
		}

		// Connect outbound (either direct or via ShadowStar proxy)
		var remoteConn io.ReadWriteCloser
		var dialErr error

		if action == routing.ActionDirect {
			remoteConn, dialErr = net.DialTimeout("tcp", destAddr, 5*time.Second)
		} else {
			// Connect via standard SOCKS5 handshake to local SOCKS5 proxy
			c, err := net.DialTimeout("tcp", ns.getSocksAddr(), 4*time.Second)
			if err == nil {
				if sErr := socks5Handshake(c, destAddr); sErr == nil {
					remoteConn = c
				} else {
					c.Close()
					dialErr = sErr
				}
			} else {
				dialErr = err
			}
		}

		if dialErr != nil {
			// Reject with RST
			ns.sendTCPPacket(dstIP, srcIP, dstPort, srcPort, ack, seq+1, TCPFlagRST|TCPFlagACK, nil)
			return
		}

		initialServerSeq := uint32(1000)
		connObj := &activeTCPConn{
			key:        key,
			targetAddr: destAddr,
			remoteConn: remoteConn,
			clientSeq:  seq + 1,
			serverSeq:  initialServerSeq + 1,
		}
		connObj.lastActive.Store(time.Now().Unix())

		ns.tcpMu.Lock()
		ns.tcpConns[key] = connObj
		ns.tcpMu.Unlock()

		// Send SYN-ACK back to client
		ns.sendTCPPacket(dstIP, srcIP, dstPort, srcPort, initialServerSeq, seq+1, TCPFlagSYN|TCPFlagACK, nil)

		// Start background reader from remote connection -> TUN
		go ns.forwardRemoteToTUN(connObj, srcIP, dstIP, srcPort, dstPort)
		return
	}

	// 2. Existing connection
	ns.tcpMu.RLock()
	connObj, exists := ns.tcpConns[key]
	ns.tcpMu.RUnlock()

	if !exists || connObj.closed.Load() {
		if flags&TCPFlagRST == 0 {
			ns.sendTCPPacket(dstIP, srcIP, dstPort, srcPort, ack, seq, TCPFlagRST|TCPFlagACK, nil)
		}
		return
	}

	connObj.lastActive.Store(time.Now().Unix())

	// Client sends data
	if len(tcpData) > 0 {
		connObj.clientSeq = seq + uint32(len(tcpData))
		_, _ = connObj.remoteConn.Write(tcpData)
		// Send ACK back
		ns.sendTCPPacket(dstIP, srcIP, dstPort, srcPort, connObj.serverSeq, connObj.clientSeq, TCPFlagACK, nil)
	}

	// Client sends FIN
	if flags&TCPFlagFIN != 0 {
		connObj.clientSeq = seq + 1
		ns.sendTCPPacket(dstIP, srcIP, dstPort, srcPort, connObj.serverSeq, connObj.clientSeq, TCPFlagACK, nil)
		connObj.closed.Store(true)
		_ = connObj.remoteConn.Close()
		ns.tcpMu.Lock()
		delete(ns.tcpConns, key)
		ns.tcpMu.Unlock()
	}

	// Client sends RST
	if flags&TCPFlagRST != 0 {
		connObj.closed.Store(true)
		_ = connObj.remoteConn.Close()
		ns.tcpMu.Lock()
		delete(ns.tcpConns, key)
		ns.tcpMu.Unlock()
	}
}

func (ns *NetStack) forwardRemoteToTUN(connObj *activeTCPConn, srcIP, dstIP net.IP, srcPort, dstPort uint16) {
	defer func() {
		connObj.closed.Store(true)
		if connObj.remoteConn != nil {
			_ = connObj.remoteConn.Close()
		}
		ns.tcpMu.Lock()
		delete(ns.tcpConns, connObj.key)
		ns.tcpMu.Unlock()
		// Send FIN to client
		ns.sendTCPPacket(dstIP, srcIP, dstPort, srcPort, connObj.serverSeq, connObj.clientSeq, TCPFlagFIN|TCPFlagACK, nil)
	}()

	buf := make([]byte, 16384)
	for ns.running.Load() && !connObj.closed.Load() {
		n, err := connObj.remoteConn.Read(buf)
		if n > 0 {
			connObj.writeMu.Lock()
			ns.sendTCPPacket(dstIP, srcIP, dstPort, srcPort, connObj.serverSeq, connObj.clientSeq, TCPFlagPSH|TCPFlagACK, buf[:n])
			connObj.serverSeq += uint32(n)
			connObj.writeMu.Unlock()
			connObj.lastActive.Store(time.Now().Unix())
		}
		if err != nil {
			break
		}
	}
}

func (ns *NetStack) reaperLoop() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ns.stopChan:
			return
		case <-ticker.C:
			now := time.Now().Unix()
			// 1. Reap idle TCP connections (> 120s)
			ns.tcpMu.Lock()
			for k, c := range ns.tcpConns {
				if now-c.lastActive.Load() > 120 || c.closed.Load() {
					c.closed.Store(true)
					if c.remoteConn != nil {
						_ = c.remoteConn.Close()
					}
					delete(ns.tcpConns, k)
				}
			}
			ns.tcpMu.Unlock()

			// 2. Reap idle UDP sessions (> 60s)
			ns.udpMu.Lock()
			for k, c := range ns.udpConns {
				if now-c.lastActive.Load() > 60 || c.closed.Load() {
					c.closed.Store(true)
					if c.directConn != nil {
						_ = c.directConn.Close()
					}
					if c.socksUDP != nil {
						_ = c.socksUDP.Close()
					}
					if c.socksTCP != nil {
						_ = c.socksTCP.Close()
					}
					delete(ns.udpConns, k)
				}
			}
			ns.udpMu.Unlock()
		}
	}
}

// Packet assembly helpers

func (ns *NetStack) sendIPv4Packet(srcIP, dstIP net.IP, proto byte, payload []byte) {
	totalLen := 20 + len(payload)
	hdr := make([]byte, 20)
	hdr[0] = 0x45 // IPv4, 20 bytes
	hdr[1] = 0    // DSCP/ECN
	binary.BigEndian.PutUint16(hdr[2:4], uint16(totalLen))
	binary.BigEndian.PutUint16(hdr[4:6], 12345) // ID
	hdr[6] = 0x40                               // Don't Fragment
	hdr[7] = 0
	hdr[8] = 64 // TTL
	hdr[9] = proto
	copy(hdr[12:16], srcIP.To4())
	copy(hdr[16:20], dstIP.To4())

	cs := checksum(hdr)
	binary.BigEndian.PutUint16(hdr[10:12], cs)

	pkt := append(hdr, payload...)
	_, _ = ns.device.Write(pkt)
}

func (ns *NetStack) sendTCPPacket(srcIP, dstIP net.IP, srcPort, dstPort uint16, seq, ack uint32, flags byte, data []byte) {
	tcpLen := 20 + len(data)
	tcpHdr := make([]byte, 20)
	binary.BigEndian.PutUint16(tcpHdr[0:2], srcPort)
	binary.BigEndian.PutUint16(tcpHdr[2:4], dstPort)
	binary.BigEndian.PutUint32(tcpHdr[4:8], seq)
	binary.BigEndian.PutUint32(tcpHdr[8:12], ack)
	tcpHdr[12] = 0x50 // Data offset: 5 32-bit words (20 bytes)
	tcpHdr[13] = flags
	binary.BigEndian.PutUint16(tcpHdr[14:16], 65535) // Window size

	// TCP Pseudo-header checksum
	pseudo := make([]byte, 12+tcpLen)
	copy(pseudo[0:4], srcIP.To4())
	copy(pseudo[4:8], dstIP.To4())
	pseudo[8] = 0
	pseudo[9] = ProtoTCP
	binary.BigEndian.PutUint16(pseudo[10:12], uint16(tcpLen))
	copy(pseudo[12:32], tcpHdr)
	if len(data) > 0 {
		copy(pseudo[32:], data)
	}

	cs := checksum(pseudo)
	binary.BigEndian.PutUint16(tcpHdr[16:18], cs)

	payload := append(tcpHdr, data...)
	ns.sendIPv4Packet(srcIP, dstIP, ProtoTCP, payload)
}

func (ns *NetStack) sendUDPPacket(srcIP, dstIP net.IP, srcPort, dstPort uint16, data []byte) {
	udpLen := 8 + len(data)
	udpHdr := make([]byte, 8)
	binary.BigEndian.PutUint16(udpHdr[0:2], srcPort)
	binary.BigEndian.PutUint16(udpHdr[2:4], dstPort)
	binary.BigEndian.PutUint16(udpHdr[4:6], uint16(udpLen))

	// UDP checksum
	pseudo := make([]byte, 12+udpLen)
	copy(pseudo[0:4], srcIP.To4())
	copy(pseudo[4:8], dstIP.To4())
	pseudo[8] = 0
	pseudo[9] = ProtoUDP
	binary.BigEndian.PutUint16(pseudo[10:12], uint16(udpLen))
	copy(pseudo[12:20], udpHdr)
	copy(pseudo[20:], data)

	cs := checksum(pseudo)
	binary.BigEndian.PutUint16(udpHdr[6:8], cs)

	payload := append(udpHdr, data...)
	ns.sendIPv4Packet(srcIP, dstIP, ProtoUDP, payload)
}

func checksum(data []byte) uint16 {
	var sum uint32
	for i := 0; i < len(data)-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}
	if len(data)%2 == 1 {
		sum += uint32(data[len(data)-1]) << 8
	}
	for (sum >> 16) > 0 {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}
	return ^uint16(sum)
}

func socks5Handshake(conn net.Conn, targetAddr string) error {
	// Greeting
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return err
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil || resp[0] != 0x05 || resp[1] != 0x00 {
		return fmt.Errorf("socks5 auth rejected")
	}

	// Request CONNECT
	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		host = targetAddr
		portStr = "80"
	}
	port, _ := strconv.Atoi(portStr)

	req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
	req = append(req, []byte(host)...)
	portBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(portBuf, uint16(port))
	req = append(req, portBuf...)

	if _, err := conn.Write(req); err != nil {
		return err
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(conn, reply[:4]); err != nil || reply[1] != 0x00 {
		return fmt.Errorf("socks5 connect failed: code=%x", reply[1])
	}
	// Read remaining bound address
	switch reply[3] {
	case 1:
		_, _ = io.ReadFull(conn, reply[4:10])
	case 3:
		dLen := make([]byte, 1)
		_, _ = io.ReadFull(conn, dLen)
		rem := make([]byte, int(dLen[0])+2)
		_, _ = io.ReadFull(conn, rem)
	case 4:
		v6 := make([]byte, 18)
		_, _ = io.ReadFull(conn, v6)
	}
	return nil
}

func parseDNSQuestion(q []byte) (string, uint16) {
	var domain string
	idx := 0
	for idx < len(q) {
		labelLen := int(q[idx])
		if labelLen == 0 {
			idx++
			break
		}
		idx++
		if idx+labelLen > len(q) {
			return "", 0
		}
		if domain != "" {
			domain += "."
		}
		domain += string(q[idx : idx+labelLen])
		idx += labelLen
	}
	if idx+4 > len(q) {
		return domain, 1
	}
	qtype := binary.BigEndian.Uint16(q[idx : idx+2])
	return domain, qtype
}

func buildSyntheticDNSResponse(txID uint16, rawQuery []byte, ipStr string) []byte {
	parsedIP := net.ParseIP(ipStr).To4()
	if parsedIP == nil {
		parsedIP = net.ParseIP("198.18.0.1").To4()
	}

	// 12 bytes header + Question section + 16 bytes Answer section
	qLen := len(rawQuery) - 12
	resp := make([]byte, 12+qLen+16)
	binary.BigEndian.PutUint16(resp[0:2], txID)
	resp[2] = 0x81 // Flags: Standard query response, No error
	resp[3] = 0x80
	binary.BigEndian.PutUint16(resp[4:6], 1) // QDCOUNT = 1
	binary.BigEndian.PutUint16(resp[6:8], 1) // ANCOUNT = 1
	binary.BigEndian.PutUint16(resp[8:10], 0)
	binary.BigEndian.PutUint16(resp[10:12], 0)

	// Copy question section
	copy(resp[12:12+qLen], rawQuery[12:])

	// Answer Section
	ansOffset := 12 + qLen
	resp[ansOffset] = 0xC0 // Pointer to Question Name
	resp[ansOffset+1] = 0x0C
	binary.BigEndian.PutUint16(resp[ansOffset+2:ansOffset+4], 1)      // TYPE A
	binary.BigEndian.PutUint16(resp[ansOffset+4:ansOffset+6], 1)      // CLASS IN
	binary.BigEndian.PutUint32(resp[ansOffset+6:ansOffset+10], 60)    // TTL: 60s
	binary.BigEndian.PutUint16(resp[ansOffset+10:ansOffset+12], 4)    // RDLENGTH: 4
	copy(resp[ansOffset+12:ansOffset+16], parsedIP)

	return resp
}

func socks5UDPAssociateHandshake(conn net.Conn) (*net.UDPAddr, error) {
	// 1. Greeting
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return nil, err
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil || resp[0] != 0x05 || resp[1] != 0x00 {
		return nil, fmt.Errorf("socks5 auth rejected")
	}

	// 2. Request UDP ASSOCIATE (CMD = 0x03) with 0.0.0.0:0
	req := []byte{0x05, 0x03, 0x00, 0x01, 0, 0, 0, 0, 0, 0}
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}

	// 3. Read reply: [VER, REP, RSV, ATYP, BND.ADDR, BND.PORT]
	repHdr := make([]byte, 4)
	if _, err := io.ReadFull(conn, repHdr); err != nil || repHdr[1] != 0x00 {
		return nil, fmt.Errorf("socks5 udp associate failed: rep=%x", repHdr[1])
	}

	var relayIP net.IP
	var relayPort uint16

	switch repHdr[3] {
	case 0x01: // IPv4
		addrBuf := make([]byte, 6)
		if _, err := io.ReadFull(conn, addrBuf); err != nil {
			return nil, err
		}
		relayIP = net.IP(addrBuf[0:4])
		relayPort = binary.BigEndian.Uint16(addrBuf[4:6])
	case 0x03: // Domain
		dLenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, dLenBuf); err != nil {
			return nil, err
		}
		domainBuf := make([]byte, int(dLenBuf[0])+2)
		if _, err := io.ReadFull(conn, domainBuf); err != nil {
			return nil, err
		}
		domain := string(domainBuf[:dLenBuf[0]])
		ips, err := net.LookupIP(domain)
		if err != nil || len(ips) == 0 {
			relayIP = net.ParseIP("127.0.0.1")
		} else {
			relayIP = ips[0]
		}
		relayPort = binary.BigEndian.Uint16(domainBuf[dLenBuf[0]:])
	case 0x04: // IPv6
		v6Buf := make([]byte, 18)
		if _, err := io.ReadFull(conn, v6Buf); err != nil {
			return nil, err
		}
		relayIP = net.IP(v6Buf[0:16])
		relayPort = binary.BigEndian.Uint16(v6Buf[16:18])
	default:
		return nil, fmt.Errorf("unsupported atyp: %d", repHdr[3])
	}

	if relayIP == nil || relayIP.IsUnspecified() {
		relayIP = net.ParseIP("127.0.0.1")
	}

	return &net.UDPAddr{IP: relayIP, Port: int(relayPort)}, nil
}

func buildSocksUDPPacket(destHost string, port uint16, payload []byte) []byte {
	var hdr []byte
	ip := net.ParseIP(destHost).To4()
	if ip != nil {
		hdr = make([]byte, 10)
		hdr[0] = 0 // RSV
		hdr[1] = 0 // RSV
		hdr[2] = 0 // FRAG
		hdr[3] = 1 // ATYP IPv4
		copy(hdr[4:8], ip)
		binary.BigEndian.PutUint16(hdr[8:10], port)
	} else {
		hdr = make([]byte, 7+len(destHost))
		hdr[0] = 0
		hdr[1] = 0
		hdr[2] = 0
		hdr[3] = 3 // ATYP Domain
		hdr[4] = byte(len(destHost))
		copy(hdr[5:5+len(destHost)], destHost)
		binary.BigEndian.PutUint16(hdr[5+len(destHost):7+len(destHost)], port)
	}
	return append(hdr, payload...)
}

func parseSocksUDPResponse(pkt []byte) []byte {
	if len(pkt) < 10 || pkt[2] != 0 {
		return nil
	}
	var hdrLen int
	switch pkt[3] {
	case 0x01: // IPv4
		hdrLen = 10
	case 0x03: // Domain
		dLen := int(pkt[4])
		hdrLen = 5 + dLen + 2
	case 0x04: // IPv6
		hdrLen = 22
	default:
		return nil
	}
	if len(pkt) < hdrLen {
		return nil
	}
	return pkt[hdrLen:]
}
