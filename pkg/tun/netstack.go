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
	inCh       chan []byte
	stopWorker chan struct{}
	closeOnce  sync.Once
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
	
	tcpConns      map[tcpConnKey]*activeTCPConn
	tcpMu         sync.RWMutex
	tcpConnecting map[tcpConnKey]bool
	connectingMu  sync.Mutex
	udpConns      map[udpConnKey]*activeUDPConn
	udpMu         sync.RWMutex
	gameMode      atomic.Bool
	running       atomic.Bool
	stopChan      chan struct{}
}

// SetGameMode toggles low-latency Game Boost mode (force proxy for all game traffic, 0 jitter)
func (ns *NetStack) SetGameMode(enabled bool) {
	ns.gameMode.Store(enabled)
	if ns.client != nil {
		ns.client.SetUDPObfuscation(!enabled)
	}
}

// IsGameMode reports whether Game Boost mode is active
func (ns *NetStack) IsGameMode() bool {
	return ns.gameMode.Load()
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
	if f == nil {
		f, _ = dns.NewFakeDNS("198.18.0.0/15")
	}
	return &NetStack{
		device:        dev,
		client:        c,
		router:        r,
		fakeDNS:       f,
		tcpConns:      make(map[tcpConnKey]*activeTCPConn),
		tcpConnecting: make(map[tcpConnKey]bool),
		udpConns:      make(map[udpConnKey]*activeUDPConn),
		stopChan:      make(chan struct{}),
	}
}

// Start begins the packet processing loop
func (ns *NetStack) Start() {
	ns.running.Store(true)
	go ns.readLoop()
	go ns.reaperLoop()
}

// closeTCPConn safely shuts down an active TCP session
func (ns *NetStack) closeTCPConn(connObj *activeTCPConn) {
	if connObj == nil {
		return
	}
	connObj.closeOnce.Do(func() {
		connObj.closed.Store(true)
		if connObj.stopWorker != nil {
			close(connObj.stopWorker)
		}
		if connObj.remoteConn != nil {
			_ = connObj.remoteConn.Close()
		}
		ns.tcpMu.Lock()
		delete(ns.tcpConns, connObj.key)
		ns.tcpMu.Unlock()
	})
}

// Stop cleanly terminates the netstack
func (ns *NetStack) Stop() {
	if !ns.running.Swap(false) {
		return
	}
	close(ns.stopChan)

	ns.tcpMu.Lock()
	conns := make([]*activeTCPConn, 0, len(ns.tcpConns))
	for _, c := range ns.tcpConns {
		conns = append(conns, c)
	}
	ns.tcpMu.Unlock()

	for _, c := range conns {
		ns.closeTCPConn(c)
	}

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
		ns.processIPv4Packet(packet)
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

// sendICMPPortUnreachable replies with RFC 792 ICMP Type 3, Code 3 (Port Unreachable)
func (ns *NetStack) sendICMPPortUnreachable(clientIP, serverIP net.IP, origPayload []byte) {
	origUDPHdr := origPayload
	if len(origUDPHdr) > 8 {
		origUDPHdr = origUDPHdr[:8]
	}

	origIPHdr := make([]byte, 20)
	origIPHdr[0] = 0x45
	binary.BigEndian.PutUint16(origIPHdr[2:4], uint16(20+len(origPayload)))
	origIPHdr[6] = 0x40 // DF
	origIPHdr[8] = 64   // TTL
	origIPHdr[9] = ProtoUDP
	copy(origIPHdr[12:16], clientIP.To4())
	copy(origIPHdr[16:20], serverIP.To4())
	binary.BigEndian.PutUint16(origIPHdr[10:12], checksum(origIPHdr))

	icmpPayload := make([]byte, 4+20+len(origUDPHdr))
	icmpPayload[0] = 3 // Type 3: Destination Unreachable
	icmpPayload[1] = 3 // Code 3: Port Unreachable
	copy(icmpPayload[8:28], origIPHdr)
	copy(icmpPayload[28:], origUDPHdr)

	cs := checksum(icmpPayload)
	binary.BigEndian.PutUint16(icmpPayload[2:4], cs)

	// Send ICMP reply from serverIP back to clientIP
	ns.sendIPv4Packet(serverIP, clientIP, ProtoICMP, icmpPayload)
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

	// Reject QUIC (UDP port 443):
	// Send ICMP Port Unreachable so YouTube app, Google services, and browsers immediately
	// fall back to TCP (HTTP/2 / HTTPS), eliminating QUIC-over-TCP latency collapse,
	// endless buffering wheels, and ISP/TSPU UDP drops.
	if dstPort == 443 {
		ns.sendICMPPortUnreachable(srcIP, dstIP, payload)
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

	// 2. New UDP session: evaluate route asynchronously
	go ns.handleNewUDP(key, srcIP, dstIP, srcPort, dstPort, udpData)
}

func (ns *NetStack) handleNewUDP(key udpConnKey, srcIP, dstIP net.IP, srcPort, dstPort uint16, udpData []byte) {
	destHost := dstIP.String()
	if ns.fakeDNS != nil && ns.fakeDNS.IsFakeIP(dstIP) {
		if realDomain, found := ns.fakeDNS.LookupDomain(dstIP); found {
			destHost = realDomain
		}
	}
	destAddr := net.JoinHostPort(destHost, strconv.Itoa(int(dstPort)))

	action := routing.ActionProxy
	if ns.gameMode.Load() {
		// Game Boost mode: enforce proxy for all game traffic to bypass regional/ISP blocks
		action = routing.ActionProxy
	} else if ns.router != nil {
		action = ns.router.Match(destAddr)
	}

	if action == routing.ActionBlock {
		return
	}

	// Double-check existing session under read lock
	ns.udpMu.RLock()
	if connObj, exists := ns.udpConns[key]; exists && !connObj.closed.Load() {
		ns.udpMu.RUnlock()
		connObj.lastActive.Store(time.Now().Unix())
		if connObj.isProxy {
			pkt := buildSocksUDPPacket(connObj.targetHost, connObj.targetPort, udpData)
			_, _ = connObj.socksUDP.Write(pkt)
		} else {
			_, _ = connObj.directConn.Write(udpData)
		}
		return
	}
	ns.udpMu.RUnlock()

	if action == routing.ActionDirect {
		dConn, err := net.DialTimeout("udp", destAddr, 3*time.Second)
		if err != nil {
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

		ns.udpMu.Lock()
		if old, exists := ns.udpConns[key]; exists && !old.closed.Load() {
			ns.udpMu.Unlock()
			_ = dConn.Close()
			_, _ = old.directConn.Write(udpData)
			return
		}
		ns.udpConns[key] = c
		ns.udpMu.Unlock()

		_, _ = dConn.Write(udpData)
		go ns.forwardDirectUDPToTUN(c, srcIP, dstIP, srcPort, dstPort)
	} else {
		// ActionProxy: SOCKS5 UDP Associate via local SOCKS5 proxy
		tcpConn, err := net.DialTimeout("tcp", ns.getSocksAddr(), 4*time.Second)
		if err != nil {
			return
		}
		relayAddr, err := socks5UDPAssociateHandshake(tcpConn)
		if err != nil {
			_ = tcpConn.Close()
			return
		}
		uConn, err := net.DialUDP("udp", nil, relayAddr)
		if err != nil {
			_ = tcpConn.Close()
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

		ns.udpMu.Lock()
		if old, exists := ns.udpConns[key]; exists && !old.closed.Load() {
			ns.udpMu.Unlock()
			_ = tcpConn.Close()
			_ = uConn.Close()
			pkt := buildSocksUDPPacket(old.targetHost, old.targetPort, udpData)
			_, _ = old.socksUDP.Write(pkt)
			return
		}
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
	domain, qtype, qLen := parseDNSQuestion(dnsData[12:])
	if domain == "" || qLen <= 0 {
		return
	}

	// 0. Immediate localhost resolution
	if domain == "localhost" {
		resp := buildSyntheticDNSResponse(txID, dnsData, qLen, "127.0.0.1")
		if resp != nil {
			ns.sendUDPPacket(dnsServerIP, clientIP, 53, clientPort, resp)
		}
		return
	}

	// 1. If A query (IPv4) and FakeDNS available, synthesize 198.18.0.x IP instantly
	if qtype == 1 && ns.fakeDNS != nil {
		fakeIP := ns.fakeDNS.Allocate(domain)
		if fakeIP != nil {
			resp := buildSyntheticDNSResponse(txID, dnsData, qLen, fakeIP.String())
			if resp != nil {
				ns.sendUDPPacket(dnsServerIP, clientIP, 53, clientPort, resp)
			}
			return
		}
	}

	// 2. For non-A queries (AAAA, Type 65 HTTPS/SVCB, etc.) in an IPv4 tunnel, respond instantly with NOERROR (0 answers)
	// so Android Bionic resolver and Happy Eyeballs immediately fall back to IPv4 without timeout or malformed types.
	if qtype != 1 {
		resp := buildEmptyDNSResponse(txID, dnsData, qLen)
		if resp != nil {
			ns.sendUDPPacket(dnsServerIP, clientIP, 53, clientPort, resp)
		}
		return
	}

	// 3. Direct upstream resolution via client or default DoH
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
				resp := buildSyntheticDNSResponse(txID, dnsData, qLen, chosenIPv4.String())
				if resp != nil {
					ns.sendUDPPacket(dnsServerIP, clientIP, 53, clientPort, resp)
				}
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

	key := tcpConnKey{
		srcIP:   srcIP.String(),
		dstIP:   dstIP.String(),
		srcPort: srcPort,
		dstPort: dstPort,
	}

	// 1. Connection initiation: SYN
	if flags&TCPFlagSYN != 0 && flags&TCPFlagACK == 0 {
		ns.tcpMu.RLock()
		_, exists := ns.tcpConns[key]
		ns.tcpMu.RUnlock()
		if exists {
			return
		}

		ns.connectingMu.Lock()
		if ns.tcpConnecting[key] {
			ns.connectingMu.Unlock()
			return // Already connecting, deduplicate retransmitted SYN
		}
		ns.tcpConnecting[key] = true
		ns.connectingMu.Unlock()

		go ns.handleTCPSYN(key, srcIP, dstIP, srcPort, dstPort, seq, ack)
		return
	}

	// 2. Existing connection: dispatch to dedicated in-order worker
	ns.tcpMu.RLock()
	connObj, exists := ns.tcpConns[key]
	ns.tcpMu.RUnlock()

	if !exists || connObj.closed.Load() {
		ns.connectingMu.Lock()
		connecting := ns.tcpConnecting[key]
		ns.connectingMu.Unlock()

		if !connecting && flags&TCPFlagRST == 0 {
			ns.sendTCPPacket(dstIP, srcIP, dstPort, srcPort, ack, seq, TCPFlagRST|TCPFlagACK, nil)
		}
		return
	}

	if !connObj.closed.Load() {
		select {
		case connObj.inCh <- payload:
		default:
			// Queue full - packet dropped (TCP backpressure throttles sender)
		}
	}
}

func (ns *NetStack) handleTCPSYN(key tcpConnKey, srcIP, dstIP net.IP, srcPort, dstPort uint16, seq, ack uint32) {
	defer func() {
		ns.connectingMu.Lock()
		delete(ns.tcpConnecting, key)
		ns.connectingMu.Unlock()
	}()

	destHost := dstIP.String()
	if ns.fakeDNS != nil && ns.fakeDNS.IsFakeIP(dstIP) {
		if dom, ok := ns.fakeDNS.LookupDomain(dstIP); ok {
			destHost = dom
		}
	}
	destAddr := net.JoinHostPort(destHost, strconv.Itoa(int(dstPort)))

	// Prevent recursive loopback self-proxying
	if dstIP.IsLoopback() || destHost == "localhost" || destHost == "127.0.0.1" {
		if dstPort == 10808 || dstPort == 10809 || dstPort == 10810 || dstPort == 10811 || dstPort == 18080 {
			ns.sendTCPPacket(dstIP, srcIP, dstPort, srcPort, ack, seq+1, TCPFlagRST|TCPFlagACK, nil)
			return
		}
	}

	// Check routing decision
	action := routing.ActionProxy
	if ns.gameMode.Load() {
		action = routing.ActionProxy
	} else if ns.router != nil {
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

	initialServerSeq := uint32(time.Now().UnixNano()&0x7FFFFFFF) + 10000
	connObj := &activeTCPConn{
		key:        key,
		targetAddr: destAddr,
		remoteConn: remoteConn,
		clientSeq:  seq + 1,
		serverSeq:  initialServerSeq + 1,
		inCh:       make(chan []byte, 256),
		stopWorker: make(chan struct{}),
	}
	connObj.lastActive.Store(time.Now().Unix())

	ns.tcpMu.Lock()
	if old, exists := ns.tcpConns[key]; exists {
		ns.closeTCPConn(old)
	}
	ns.tcpConns[key] = connObj
	ns.tcpMu.Unlock()

	// Send SYN-ACK back to client (with MSS 1360, Window Scale 7, SACK Permitted options)
	ns.sendTCPPacket(dstIP, srcIP, dstPort, srcPort, initialServerSeq, seq+1, TCPFlagSYN|TCPFlagACK, nil)

	// Start per-connection in-order worker for client packets
	go ns.tcpConnectionInWorker(connObj, srcIP, dstIP, srcPort, dstPort)

	// Start background reader from remote connection -> TUN
	go ns.forwardRemoteToTUN(connObj, srcIP, dstIP, srcPort, dstPort)
}

func (ns *NetStack) tcpConnectionInWorker(connObj *activeTCPConn, srcIP, dstIP net.IP, srcPort, dstPort uint16) {
	for {
		select {
		case <-ns.stopChan:
			return
		case <-connObj.stopWorker:
			return
		case payload, ok := <-connObj.inCh:
			if !ok || connObj.closed.Load() {
				return
			}
			ns.handleTCPData(connObj, srcIP, dstIP, srcPort, dstPort, payload)
		}
	}
}

func (ns *NetStack) handleTCPData(connObj *activeTCPConn, srcIP, dstIP net.IP, srcPort, dstPort uint16, payload []byte) {
	if len(payload) < 20 {
		return
	}
	seq := binary.BigEndian.Uint32(payload[4:8])
	dataOffset := int(payload[12]>>4) * 4
	flags := payload[13]

	if len(payload) < dataOffset {
		return
	}
	tcpData := payload[dataOffset:]

	connObj.lastActive.Store(time.Now().Unix())

	hasData := len(tcpData) > 0
	endSeq := seq + uint32(len(tcpData))

	// Client sends data
	if hasData {
		if endSeq > connObj.clientSeq {
			offset := uint32(0)
			if seq < connObj.clientSeq {
				offset = connObj.clientSeq - seq
			}
			if offset < uint32(len(tcpData)) {
				_, _ = connObj.remoteConn.Write(tcpData[offset:])
			}
			connObj.clientSeq = endSeq
		}
		// Send ACK back
		ns.sendTCPPacket(dstIP, srcIP, dstPort, srcPort, connObj.serverSeq, connObj.clientSeq, TCPFlagACK, nil)
	}

	// Client sends FIN
	if flags&TCPFlagFIN != 0 {
		if hasData {
			connObj.clientSeq = endSeq + 1
		} else {
			connObj.clientSeq = seq + 1
		}
		ns.sendTCPPacket(dstIP, srcIP, dstPort, srcPort, connObj.serverSeq, connObj.clientSeq, TCPFlagACK, nil)
		ns.closeTCPConn(connObj)
		return
	}

	// Client sends RST
	if flags&TCPFlagRST != 0 {
		ns.closeTCPConn(connObj)
		return
	}
}

func (ns *NetStack) forwardRemoteToTUN(connObj *activeTCPConn, srcIP, dstIP net.IP, srcPort, dstPort uint16) {
	defer func() {
		// Send FIN to client
		ns.sendTCPPacket(dstIP, srcIP, dstPort, srcPort, connObj.serverSeq, connObj.clientSeq, TCPFlagFIN|TCPFlagACK, nil)
		ns.closeTCPConn(connObj)
	}()

	buf := make([]byte, 32768)
	for ns.running.Load() && !connObj.closed.Load() {
		n, err := connObj.remoteConn.Read(buf)
		if n > 0 {
			connObj.writeMu.Lock()
			// MSS chunking: Android TUN device MTU is 1500 (IPv4 hdr 20 + TCP hdr 20 = 40 bytes overhead).
			// If payload > 1360 bytes, kernel/TUN driver drops oversized packets.
			const maxMSS = 1360
			offset := 0
			for offset < n {
				chunkSize := n - offset
				if chunkSize > maxMSS {
					chunkSize = maxMSS
				}
				flags := byte(TCPFlagACK)
				if offset+chunkSize == n {
					flags |= TCPFlagPSH
				}
				ns.sendTCPPacket(dstIP, srcIP, dstPort, srcPort, connObj.serverSeq, connObj.clientSeq, flags, buf[offset:offset+chunkSize])
				connObj.serverSeq += uint32(chunkSize)
				offset += chunkSize
			}
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
			var toClose []*activeTCPConn
			ns.tcpMu.RLock()
			for _, c := range ns.tcpConns {
				if now-c.lastActive.Load() > 120 || c.closed.Load() {
					toClose = append(toClose, c)
				}
			}
			ns.tcpMu.RUnlock()

			for _, c := range toClose {
				ns.closeTCPConn(c)
			}

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
	var tcpHdrLen int
	var tcpHdr []byte
	isSYN := flags&TCPFlagSYN != 0

	if isSYN {
		// Include standard TCP options for SYN and SYN-ACK:
		// Total 12 bytes of options -> 32-byte header (Data Offset = 8 words = 0x80)
		tcpHdrLen = 32
		tcpHdr = make([]byte, tcpHdrLen)
		tcpHdr[12] = 0x80 // Data offset: 8 32-bit words (32 bytes)
		// Option 1: MSS (Kind 2, Length 4, Value 1360 = 0x0550)
		tcpHdr[20] = 0x02
		tcpHdr[21] = 0x04
		binary.BigEndian.PutUint16(tcpHdr[22:24], 1360)
		// Option 2: NOP
		tcpHdr[24] = 0x01
		// Option 3: Window Scale (Kind 3, Length 3, Shift 7 -> 128x multiplier)
		tcpHdr[25] = 0x03
		tcpHdr[26] = 0x03
		tcpHdr[27] = 0x07
		// Option 4: SACK Permitted (Kind 4, Length 2)
		tcpHdr[28] = 0x04
		tcpHdr[29] = 0x02
		// Option 5: NOP padding to 4-byte boundary
		tcpHdr[30] = 0x01
		tcpHdr[31] = 0x01
	} else {
		tcpHdrLen = 20
		tcpHdr = make([]byte, tcpHdrLen)
		tcpHdr[12] = 0x50 // Data offset: 5 32-bit words (20 bytes)
	}

	binary.BigEndian.PutUint16(tcpHdr[0:2], srcPort)
	binary.BigEndian.PutUint16(tcpHdr[2:4], dstPort)
	binary.BigEndian.PutUint32(tcpHdr[4:8], seq)
	binary.BigEndian.PutUint32(tcpHdr[8:12], ack)
	tcpHdr[13] = flags
	binary.BigEndian.PutUint16(tcpHdr[14:16], 65535) // Window size (65535 * 128 = 8.3 MB with scale 7)

	tcpLen := tcpHdrLen + len(data)
	// TCP Pseudo-header checksum
	pseudo := make([]byte, 12+tcpLen)
	copy(pseudo[0:4], srcIP.To4())
	copy(pseudo[4:8], dstIP.To4())
	pseudo[8] = 0
	pseudo[9] = ProtoTCP
	binary.BigEndian.PutUint16(pseudo[10:12], uint16(tcpLen))
	copy(pseudo[12:12+tcpHdrLen], tcpHdr)
	if len(data) > 0 {
		copy(pseudo[12+tcpHdrLen:], data)
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
	if cs == 0 {
		cs = 0xFFFF
	}
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

	var req []byte
	ip := net.ParseIP(host).To4()
	if ip != nil {
		// ATYP IPv4 (0x01)
		req = make([]byte, 10)
		req[0] = 0x05
		req[1] = 0x01 // CONNECT
		req[2] = 0x00 // RSV
		req[3] = 0x01 // ATYP IPv4
		copy(req[4:8], ip)
		binary.BigEndian.PutUint16(req[8:10], uint16(port))
	} else {
		// ATYP Domain (0x03)
		req = make([]byte, 7+len(host))
		req[0] = 0x05
		req[1] = 0x01 // CONNECT
		req[2] = 0x00 // RSV
		req[3] = 0x03 // ATYP Domain
		req[4] = byte(len(host))
		copy(req[5:5+len(host)], host)
		binary.BigEndian.PutUint16(req[5+len(host):7+len(host)], uint16(port))
	}

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

func parseDNSQuestion(q []byte) (string, uint16, int) {
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
			return "", 0, 0
		}
		if domain != "" {
			domain += "."
		}
		domain += string(q[idx : idx+labelLen])
		idx += labelLen
	}
	if idx+4 > len(q) {
		return domain, 1, len(q)
	}
	qtype := binary.BigEndian.Uint16(q[idx : idx+2])
	// Question section ends at idx + 4 (QTYPE 2 bytes + QCLASS 2 bytes)
	return domain, qtype, idx + 4
}

func buildSyntheticDNSResponse(txID uint16, rawQuery []byte, qLen int, ipStr string) []byte {
	parsedIP := net.ParseIP(ipStr).To4()
	if parsedIP == nil {
		parsedIP = net.ParseIP("198.18.0.1").To4()
	}

	if qLen <= 0 || 12+qLen > len(rawQuery) {
		qLen = len(rawQuery) - 12
	}
	if qLen <= 0 {
		return nil
	}

	// 12 bytes header + Question section + 16 bytes Answer section
	resp := make([]byte, 12+qLen+16)
	binary.BigEndian.PutUint16(resp[0:2], txID)
	resp[2] = 0x81 // Flags: Standard query response, No error
	resp[3] = 0x80 // RA (Recursion Available)
	binary.BigEndian.PutUint16(resp[4:6], 1) // QDCOUNT = 1
	binary.BigEndian.PutUint16(resp[6:8], 1) // ANCOUNT = 1
	binary.BigEndian.PutUint16(resp[8:10], 0)
	binary.BigEndian.PutUint16(resp[10:12], 0) // ARCOUNT = 0

	// Copy question section ONLY (strips any trailing EDNS0 / OPT pseudo-records)
	copy(resp[12:12+qLen], rawQuery[12:12+qLen])

	// Answer Section placed immediately after Question section
	ansOffset := 12 + qLen
	resp[ansOffset] = 0xC0 // Pointer to Question Name at offset 12
	resp[ansOffset+1] = 0x0C
	binary.BigEndian.PutUint16(resp[ansOffset+2:ansOffset+4], 1)   // TYPE A
	binary.BigEndian.PutUint16(resp[ansOffset+4:ansOffset+6], 1)   // CLASS IN
	binary.BigEndian.PutUint32(resp[ansOffset+6:ansOffset+10], 60) // TTL: 60s
	binary.BigEndian.PutUint16(resp[ansOffset+10:ansOffset+12], 4) // RDLENGTH: 4
	copy(resp[ansOffset+12:ansOffset+16], parsedIP)

	return resp
}

func buildEmptyDNSResponse(txID uint16, rawQuery []byte, qLen int) []byte {
	if qLen <= 0 || 12+qLen > len(rawQuery) {
		qLen = len(rawQuery) - 12
	}
	if qLen <= 0 {
		return nil
	}
	// Standard DNS header (12 bytes) + Question section, ANCOUNT = 0, NOERROR
	resp := make([]byte, 12+qLen)
	binary.BigEndian.PutUint16(resp[0:2], txID)
	resp[2] = 0x81 // Flags: Standard query response, No error
	resp[3] = 0x80 // RA (Recursion Available)
	binary.BigEndian.PutUint16(resp[4:6], 1) // QDCOUNT = 1
	binary.BigEndian.PutUint16(resp[6:8], 0) // ANCOUNT = 0
	binary.BigEndian.PutUint16(resp[8:10], 0)
	binary.BigEndian.PutUint16(resp[10:12], 0)
	copy(resp[12:12+qLen], rawQuery[12:12+qLen])
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
