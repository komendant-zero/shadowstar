package client

import (
	"bufio"
	"bytes"
	"crypto/ecdh"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shadowstar/shadowstar/pkg/protocol"
	"github.com/shadowstar/shadowstar/pkg/routing"
)

// ServerProfile holds connection settings for a ShadowStar node
type ServerProfile struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	ServerAddr    string   `json:"server_addr"`    // e.g. "1.2.3.4:443"
	MasterKey     string   `json:"master_key"`     // secret
	SNI           string   `json:"sni"`            // e.g. "www.microsoft.com"
	EnableChunker bool     `json:"enable_chunker"` // TCP fragmentation
	PaddingLen    int      `json:"padding_len"`    // Dynamic padding (16-256)
	Ping          int      `json:"ping"`           // ms latency
	TimeOffset    int64    `json:"time_offset"`    // clock drift correction in seconds
	Country       string   `json:"country,omitempty"` // e.g. "NL", "DE", "US"
	GroupID       string   `json:"group_id,omitempty"` // category / subscription group ID
	SNIPool       []string `json:"sni_pool,omitempty"`
	Fingerprint   string   `json:"fingerprint,omitempty"`
	Transport     string   `json:"transport,omitempty"` // "tcp" (default), "udp", "auto"
}

// SubscriptionGroup represents a category of servers or an external subscription
type SubscriptionGroup struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url,omitempty"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
}

// Config holds client operational settings
type Config struct {
	SocksAddr       string              `json:"socks_addr"` // "127.0.0.1:10808"
	HttpAddr        string              `json:"http_addr"`  // "127.0.0.1:10809"
	SubscriptionURL string              `json:"subscription_url,omitempty"`
	Groups          []SubscriptionGroup `json:"groups,omitempty"`
	ActiveGroupID   string              `json:"active_group_id,omitempty"`
	Servers         []ServerProfile     `json:"servers"`
	ActiveID        string              `json:"active_id"`
	Routing         *routing.Config     `json:"routing,omitempty"`
	Whitelist       []string            `json:"whitelist,omitempty"`
	Blacklist       []string            `json:"blacklist,omitempty"`
}

// EnsureGroups ensures there is at least one group and migrates ungrouped servers
func (c *Config) EnsureGroups() {
	if len(c.Groups) == 0 {
		defaultName := "Основная"
		if c.SubscriptionURL != "" {
			defaultName = "Подписка"
		}
		c.Groups = []SubscriptionGroup{
			{
				ID:        "group-default",
				Name:      defaultName,
				URL:       c.SubscriptionURL,
				UpdatedAt: time.Now().Unix(),
			},
		}
		c.ActiveGroupID = "group-default"
	}
	if c.ActiveGroupID == "" && len(c.Groups) > 0 {
		c.ActiveGroupID = c.Groups[0].ID
	}
	defaultID := c.Groups[0].ID
	for i := range c.Servers {
		if c.Servers[i].GroupID == "" {
			c.Servers[i].GroupID = defaultID
		}
	}
}

// GetActiveGroup returns pointer to the currently active group
func (c *Config) GetActiveGroup() *SubscriptionGroup {
	c.EnsureGroups()
	for i := range c.Groups {
		if c.Groups[i].ID == c.ActiveGroupID {
			return &c.Groups[i]
		}
	}
	if len(c.Groups) > 0 {
		c.ActiveGroupID = c.Groups[0].ID
		return &c.Groups[0]
	}
	return nil
}

// GetServersInGroup returns slice of servers belonging to groupID
func (c *Config) GetServersInGroup(groupID string) []ServerProfile {
	c.EnsureGroups()
	var res []ServerProfile
	for _, s := range c.Servers {
		if s.GroupID == groupID {
			res = append(res, s)
		}
	}
	return res
}

// GetGroupServerCount returns how many servers are in groupID
func (c *Config) GetGroupServerCount(groupID string) int {
	count := 0
	for _, s := range c.Servers {
		if s.GroupID == groupID {
			count++
		}
	}
	return count
}

// AddGroup creates a new group and sets it as active
func (c *Config) AddGroup(name, subURL string) *SubscriptionGroup {
	c.EnsureGroups()
	if name == "" {
		name = fmt.Sprintf("Группа %d", len(c.Groups)+1)
	}
	grp := SubscriptionGroup{
		ID:        fmt.Sprintf("group-%d", time.Now().UnixNano()),
		Name:      name,
		URL:       subURL,
		UpdatedAt: time.Now().Unix(),
	}
	c.Groups = append(c.Groups, grp)
	c.ActiveGroupID = grp.ID
	return &c.Groups[len(c.Groups)-1]
}

// UpdateGroup modifies group name and URL
func (c *Config) UpdateGroup(groupID, newName, newURL string) bool {
	c.EnsureGroups()
	for i := range c.Groups {
		if c.Groups[i].ID == groupID {
			if newName != "" {
				c.Groups[i].Name = newName
			}
			c.Groups[i].URL = newURL
			c.Groups[i].UpdatedAt = time.Now().Unix()
			return true
		}
	}
	return false
}

// DeleteGroup removes a group and all its servers
func (c *Config) DeleteGroup(groupID string) bool {
	c.EnsureGroups()
	if len(c.Groups) <= 1 {
		// Cannot delete the only remaining group; reset it instead
		c.Groups[0].Name = "Основная"
		c.Groups[0].URL = ""
		c.Groups[0].UpdatedAt = time.Now().Unix()
		var remaining []ServerProfile
		for _, s := range c.Servers {
			if s.GroupID != groupID {
				remaining = append(remaining, s)
			}
		}
		c.Servers = remaining
		return true
	}

	// Remove servers belonging to this group
	var remaining []ServerProfile
	for _, s := range c.Servers {
		if s.GroupID != groupID {
			remaining = append(remaining, s)
		}
	}
	c.Servers = remaining

	// Remove group from slice
	var newGroups []SubscriptionGroup
	for _, g := range c.Groups {
		if g.ID != groupID {
			newGroups = append(newGroups, g)
		}
	}
	c.Groups = newGroups

	if c.ActiveGroupID == groupID {
		c.ActiveGroupID = c.Groups[0].ID
	}
	return true
}

// Stats holds real-time network telemetry
type Stats struct {
	BytesUp       uint64  `json:"bytes_up"`
	BytesDown     uint64  `json:"bytes_down"`
	SpeedUp       float64 `json:"speed_up"`   // bytes/sec
	SpeedDown     float64 `json:"speed_down"` // bytes/sec
	ActiveConns   int64   `json:"active_conns"`
	Connected     bool    `json:"connected"`
	ConnectedTime int64   `json:"connected_time"` // unix seconds
	Latency       int     `json:"latency"`        // ms
}

// Client manages local proxy listeners and remote encrypted multiplexed sessions
type Client struct {
	profile       ServerProfile
	socksAddr     string
	httpAddr      string
	masterKey     []byte
	legacyKey     []byte
	socksListener net.Listener
	httpListener  net.Listener
	session       *protocol.MuxSession
	muSession     sync.Mutex
	mu            sync.RWMutex
	running       bool

	// Telemetry
	bytesUp       atomic.Uint64
	bytesDown     atomic.Uint64
	lastBytesUp   uint64
	lastBytesDown uint64
	speedUp       atomic.Uint64
	speedDown     atomic.Uint64
	activeConns   atomic.Int64
	startTime     time.Time
	latency       atomic.Int64

	// Stealth & Resilience
	sniMgr        *SNIPoolManager
	udpJitterMax  time.Duration
	udpObfs       bool
	servers       []ServerProfile
	activeGroupID string
	healthMonitor HealthMonitor
	tunnelDialer  func(destAddr string) (net.Conn, error)
	router        routing.Engine

	// Log stream
	logChan chan string
	logMu   sync.Mutex
	logs    []string
}

type customReadWriteCloser struct {
	io.Reader
	io.Writer
	io.Closer
}

func isLocalHost(hostPort string) bool {
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		host = hostPort
	}
	host = strings.TrimSpace(strings.ToLower(host))
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasPrefix(host, "127.")
}

func sanitizeSNI(sni string) string {
	sni = strings.TrimSpace(sni)
	if strings.Contains(sni, ":") {
		h, _, err := net.SplitHostPort(sni)
		if err == nil && h != "" {
			return h
		} else if idx := strings.Index(sni, ":"); idx != -1 {
			return sni[:idx]
		}
	}
	return sni
}

func sanitizeProfile(p *ServerProfile) {
	p.ServerAddr = strings.TrimSpace(p.ServerAddr)
	p.MasterKey = strings.TrimSpace(p.MasterKey)
	p.SNI = sanitizeSNI(p.SNI)
	if p.ServerAddr != "" && !strings.Contains(p.ServerAddr, ":") {
		p.ServerAddr = p.ServerAddr + ":4433"
	}
	if p.SNI == "" {
		p.SNI = "www.microsoft.com"
	}

	var cleanPool []string
	seen := make(map[string]bool)
	for _, s := range p.SNIPool {
		clean := sanitizeSNI(s)
		if clean != "" && !seen[clean] {
			cleanPool = append(cleanPool, clean)
			seen[clean] = true
		}
	}
	if !seen[p.SNI] && p.SNI != "" {
		cleanPool = append([]string{p.SNI}, cleanPool...)
		seen[p.SNI] = true
	}
	if len(cleanPool) == 0 {
		cleanPool = []string{p.SNI}
	}
	p.SNIPool = cleanPool

	if p.Fingerprint == "" {
		p.Fingerprint = "chrome_133"
	}
	if p.PaddingLen == 0 {
		p.PaddingLen = 64
	}
}

// NewClient initializes a new proxy client
func NewClient(profile ServerProfile, socksAddr, httpAddr string) (*Client, error) {
	sanitizeProfile(&profile)

	if socksAddr == "" {
		socksAddr = "127.0.0.1:10808"
	}
	if httpAddr == "" {
		httpAddr = "127.0.0.1:10809"
	}

	primaryKey, legacyKey := protocol.NormalizeMasterKey(profile.MasterKey)

	defRouter, _ := routing.NewEngine(routing.DefaultConfig())

	c := &Client{
		profile:      profile,
		socksAddr:    socksAddr,
		httpAddr:     httpAddr,
		masterKey:    primaryKey,
		legacyKey:    legacyKey,
		router:       defRouter,
		logChan:      make(chan string, 100),
		logs:         make([]string, 0, 100),
		sniMgr:       NewSNIPoolManager(profile.SNIPool, profile.SNI),
		udpJitterMax: 25 * time.Millisecond,
		udpObfs:      true,
	}

	return c, nil
}

// SetRouter sets the routing engine for destination routing decisions
func (c *Client) SetRouter(r routing.Engine) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.router = r
}

// GetRouter returns the active routing engine
func (c *Client) GetRouter() routing.Engine {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.router
}

// Start launches the local SOCKS5 and HTTP proxy servers
func (c *Client) Start() error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return nil
	}
	c.running = true
	c.startTime = time.Now()
	c.mu.Unlock()

	// 1. SOCKS5 Listener
	sl, err := net.Listen("tcp", c.socksAddr)
	if err != nil {
		return fmt.Errorf("failed to bind socks5 to %s: %w", c.socksAddr, err)
	}
	c.socksListener = sl

	// 2. HTTP Listener
	hl, err := net.Listen("tcp", c.httpAddr)
	if err != nil {
		sl.Close()
		return fmt.Errorf("failed to bind http proxy to %s: %w", c.httpAddr, err)
	}
	c.httpListener = hl

	c.Log(fmt.Sprintf("[Client] SOCKS5 listening on %s | HTTP proxy on %s", c.socksAddr, c.httpAddr))
	c.Log(fmt.Sprintf("[Tunnel] Connecting to server %s (SNI Masquerade: %s, Chunker: %v)", c.profile.ServerAddr, c.profile.SNI, c.profile.EnableChunker))

	go c.runSpeedMonitor()
	go c.runPingMonitor()
	go c.acceptSocksLoop()
	go c.acceptHttpLoop()

	return nil
}

// Stop shuts down listeners and the proxy session
func (c *Client) Stop() error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	c.running = false
	hm := c.healthMonitor
	c.healthMonitor = nil
	c.mu.Unlock()

	if hm != nil {
		hm.Stop()
	}

	if c.socksListener != nil {
		c.socksListener.Close()
	}
	if c.httpListener != nil {
		c.httpListener.Close()
	}

	c.muSession.Lock()
	if c.session != nil {
		c.session.Close()
		c.session = nil
	}
	c.muSession.Unlock()

	c.Log("[Client] Proxy engine stopped")
	return nil
}

// UpdateProfile updates the remote server profile and reconnects
func (c *Client) GetProfile() ServerProfile {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.profile
}

func (c *Client) UpdateProfile(p ServerProfile) {
	sanitizeProfile(&p)
	c.mu.Lock()
	c.profile = p
	primaryKey, legacyKey := protocol.NormalizeMasterKey(p.MasterKey)
	c.masterKey = primaryKey
	c.legacyKey = legacyKey
	if c.sniMgr != nil {
		c.sniMgr.SetPool(p.SNIPool, p.SNI)
	} else {
		c.sniMgr = NewSNIPoolManager(p.SNIPool, p.SNI)
	}
	c.mu.Unlock()

	c.muSession.Lock()
	if c.session != nil {
		c.session.Close()
		c.session = nil
	}
	c.muSession.Unlock()
	c.Log(fmt.Sprintf("[Client] Switched active server to: %s (%s)", p.Name, p.ServerAddr))
}

// SetTransport changes the active server's transport mode (tcp, udp, auto) and reconnects
func (c *Client) SetTransport(trans string) {
	trans = strings.ToLower(strings.TrimSpace(trans))
	if trans != "udp" && trans != "auto" {
		trans = "tcp"
	}
	c.mu.Lock()
	c.profile.Transport = trans
	c.mu.Unlock()

	c.muSession.Lock()
	if c.session != nil {
		c.session.Close()
		c.session = nil
	}
	c.muSession.Unlock()
	c.Log(fmt.Sprintf("[Client] Transport mode set to %s. Reconnecting tunnel...", strings.ToUpper(trans)))
}

// GetTransport returns the active server's transport mode
func (c *Client) GetTransport() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.profile.Transport == "" {
		return "tcp"
	}
	return c.profile.Transport
}

// dialTransport connects to the remote server using the configured transport (TCP, UDP, or Auto)
func (c *Client) dialTransport(addr string) (net.Conn, string, error) {
	trans := strings.ToLower(strings.TrimSpace(c.profile.Transport))
	if trans == "" {
		trans = "tcp"
	}

	switch trans {
	case "udp":
		conn, err := protocol.DialUDP(addr)
		return conn, "UDP", err
	case "auto":
		// Auto mode: attempt fast TCP first (2.5s timeout)
		conn, err := net.DialTimeout("tcp", addr, 2500*time.Millisecond)
		if err == nil {
			return conn, "TCP (Auto)", nil
		}
		c.Log(fmt.Sprintf("[Transport] TCP attempt failed (%v), automatically switching to UDP...", err))
		conn, uErr := protocol.DialUDP(addr)
		if uErr == nil {
			return conn, "UDP (Auto-Fallback)", nil
		}
		return nil, "Auto", fmt.Errorf("both TCP (%v) and UDP (%v) failed", err, uErr)
	case "tcp":
		fallthrough
	default:
		conn, err := net.DialTimeout("tcp", addr, 8*time.Second)
		return conn, "TCP", err
	}
}

func (c *Client) getSession() (*protocol.MuxSession, error) {
	c.muSession.Lock()
	defer c.muSession.Unlock()

	if c.session != nil && !c.session.IsClosed() {
		return c.session, nil
	}

	c.mu.RLock()
	sniMgr := c.sniMgr
	poolLen := len(c.profile.SNIPool)
	c.mu.RUnlock()

	if sniMgr == nil {
		sniMgr = NewSNIPoolManager(c.profile.SNIPool, c.profile.SNI)
		c.mu.Lock()
		c.sniMgr = sniMgr
		c.mu.Unlock()
	}

	attempts := poolLen
	if attempts < 2 {
		attempts = 2
	}
	if attempts > 4 {
		attempts = 4
	}

	var lastErr error

	for att := 0; att < attempts; att++ {
		selectedSNI := sniMgr.SelectSNI()
		conn, usedTrans, err := c.dialTransport(c.profile.ServerAddr)
		c.Log(fmt.Sprintf("[Handshake] Initiating Camouflage Handshake (%s) -> %s (SNI: %s, Attempt %d/%d)",
			usedTrans, c.profile.ServerAddr, selectedSNI, att+1, attempts))
		start := time.Now()

		if err != nil {
			sniMgr.ReportFailure(selectedSNI, err)
			lastErr = fmt.Errorf("failed to connect via %s: %w", usedTrans, err)
			time.Sleep(50 * time.Millisecond)
			continue
		}

		// Perform TLS 1.3 Mimicry Handshake (with key fallback & auto clock-drift tolerance)
		var clientPriv *ecdh.PrivateKey
		var serverPub []byte
		var usedKey []byte
		candidates := []int64{c.profile.TimeOffset, 170, 180, 0, 120, 240, -120}

		keyOptions := [][]byte{c.masterKey}
		if len(c.legacyKey) > 0 {
			keyOptions = append(keyOptions, c.legacyKey)
		}

		var lastHandshakeErr error
		for kIdx, tryKey := range keyOptions {
			if clientPriv != nil {
				break
			}
			for idx, off := range candidates {
				if clientPriv != nil {
					break
				}
				if (idx > 0 || kIdx > 0) && conn != nil {
					conn.Close()
					conn, usedTrans, err = c.dialTransport(c.profile.ServerAddr)
					if err != nil {
						lastHandshakeErr = fmt.Errorf("reconnect failed: %w", err)
						break
					}
				}

				cp, sp, hErr := protocol.PerformClientHandshake(conn, tryKey, selectedSNI, c.profile.EnableChunker, c.profile.PaddingLen, off)
				if hErr == nil && sp != nil {
					clientPriv = cp
					serverPub = sp
					usedKey = tryKey
					c.profile.TimeOffset = off
					break
				} else {
					lastHandshakeErr = hErr
				}
			}
		}

		if clientPriv == nil || serverPub == nil {
			if conn != nil {
				conn.Close()
			}
			sniMgr.ReportFailure(selectedSNI, lastHandshakeErr)
			lastErr = fmt.Errorf("handshake failed with SNI %s: %v", selectedSNI, lastHandshakeErr)
			time.Sleep(50 * time.Millisecond)
			continue
		}

		// Compute ECDH shared secret
		sharedSecret, err := protocol.ComputeECDH(clientPriv, serverPub)
		if err != nil {
			conn.Close()
			sniMgr.ReportFailure(selectedSNI, err)
			lastErr = fmt.Errorf("ecdh failed: %w", err)
			continue
		}

		// Derive Bidirectional Session Keys using the successfully authenticated key
		clientKey, serverKey, clientIV, serverIV, err := protocol.DeriveSessionKeys(sharedSecret, usedKey)
		if err != nil {
			conn.Close()
			sniMgr.ReportFailure(selectedSNI, err)
			lastErr = fmt.Errorf("derive keys failed: %w", err)
			continue
		}

		// Client writes with clientKey, reads with serverKey
		frameConn, err := protocol.NewSecureFrameConn(conn, clientKey, clientIV, serverKey, serverIV)
		if err != nil {
			conn.Close()
			sniMgr.ReportFailure(selectedSNI, err)
			lastErr = fmt.Errorf("frameConn failed: %w", err)
			continue
		}

		sess := protocol.NewMuxSession(frameConn, conn, true)
		c.session = sess
		sniMgr.ReportSuccess(selectedSNI)
		latencyMs := int(time.Since(start).Milliseconds())
		c.latency.Store(int64(latencyMs))
		c.Log(fmt.Sprintf("[Handshake-Success] Secure tunnel established via %s (SNI: %s)! Latency: %d ms", usedTrans, selectedSNI, latencyMs))

		return sess, nil
	}

	return nil, lastErr
}

func (c *Client) acceptSocksLoop() {
	for {
		conn, err := c.socksListener.Accept()
		if err != nil {
			return
		}
		go c.handleSocksClient(conn)
	}
}

func (c *Client) openStreamWithRetry(destAddr string) (*protocol.MuxStream, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		sess, err := c.getSession()
		if err != nil {
			lastErr = err
			time.Sleep(150 * time.Millisecond)
			continue
		}

		stream, err := sess.OpenStream()
		if err != nil {
			c.muSession.Lock()
			if c.session == sess {
				c.session = nil
			}
			c.muSession.Unlock()
			lastErr = err
			continue
		}

		if err := protocol.WriteTargetAddress(stream, protocol.NetTCP, destAddr); err != nil {
			stream.Close()
			c.muSession.Lock()
			if c.session == sess {
				c.session = nil
			}
			c.muSession.Unlock()
			lastErr = err
			continue
		}

		return stream, nil
	}
	return nil, lastErr
}

func (c *Client) handleSocksClient(conn net.Conn) {
	defer conn.Close()
	c.activeConns.Add(1)
	defer c.activeConns.Add(-1)

	// SOCKS5 Greeting
	buf := make([]byte, 258)
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	if buf[0] != 0x05 {
		return
	}
	nMethods := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:nMethods]); err != nil {
		return
	}

	// Respond: SOCKS5, No Auth Required (0x00)
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// Read Request
	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return
	}
	if buf[1] == 0x03 { // SOCKS5 UDP ASSOCIATE
		c.handleSocksUDP(conn)
		return
	}
	if buf[1] != 0x01 { // CMD != CONNECT
		// Command not supported
		_, _ = conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	atyp := buf[3]
	var host string
	switch atyp {
	case protocol.AtypIPv4:
		ipBuf := make([]byte, 4)
		if _, err := io.ReadFull(conn, ipBuf); err != nil {
			return
		}
		host = net.IP(ipBuf).String()
	case protocol.AtypDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return
		}
		domainBuf := make([]byte, int(lenBuf[0]))
		if _, err := io.ReadFull(conn, domainBuf); err != nil {
			return
		}
		host = string(domainBuf)
	case protocol.AtypIPv6:
		ipBuf := make([]byte, 16)
		if _, err := io.ReadFull(conn, ipBuf); err != nil {
			return
		}
		host = net.IP(ipBuf).String()
	default:
		return
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(portBuf)
	destAddr := net.JoinHostPort(host, strconv.Itoa(int(port)))

	// Route evaluation
	c.mu.RLock()
	router := c.router
	c.mu.RUnlock()

	var action routing.Action = routing.ActionProxy
	if isLocalHost(destAddr) {
		action = routing.ActionDirect
	} else if router != nil {
		action = router.Match(destAddr)
	}

	switch action {
	case routing.ActionBlock:
		_, _ = conn.Write([]byte{0x05, 0x02, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // Connection not allowed
		return
	case routing.ActionDirect:
		_, portStr, _ := net.SplitHostPort(destAddr)
		if isLocalHost(destAddr) && (portStr == "10809" || portStr == "10808" || portStr == "10810" || portStr == "10811" || portStr == "18080") {
			_, _ = conn.Write([]byte{0x05, 0x02, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
			return
		}
		localConn, err := net.DialTimeout("tcp", destAddr, 5*time.Second)
		if err != nil {
			_, _ = conn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
			return
		}
		defer localConn.Close()
		if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
			return
		}
		c.pipeBidirectional(conn, localConn)
		return
	case routing.ActionProxy:
		// Open multiplexed stream with retry
		stream, err := c.openStreamWithRetry(destAddr)
		if err != nil {
			c.Log(fmt.Sprintf("[Error] Tunnel unavailable for %s: %v", destAddr, err))
			_, _ = conn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
			return
		}
		defer stream.Close()

		if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
			return
		}
		c.pipeBidirectional(conn, stream)
	}
}

type clientUDPSession struct {
	stream     *protocol.MuxStream
	header     []byte
	lastActive atomic.Int64
	closed     atomic.Bool
	writeMu    sync.Mutex
}

func (c *Client) handleSocksUDP(tcpConn net.Conn) {
	// Listen on local UDP relay
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		_, _ = tcpConn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer udpConn.Close()

	localAddr := udpConn.LocalAddr().(*net.UDPAddr)
	port := uint16(localAddr.Port)

	// Send success reply to client with the UDP relay port
	resp := []byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, byte(port >> 8), byte(port & 0xff)}
	if _, err := tcpConn.Write(resp); err != nil {
		return
	}

	var sessionMu sync.Mutex
	sessions := make(map[string]*clientUDPSession)

	// Close all sessions on exit
	defer func() {
		sessionMu.Lock()
		for k, s := range sessions {
			s.closed.Store(true)
			_ = s.stream.Close()
			delete(sessions, k)
		}
		sessionMu.Unlock()
	}()

	// Reaper goroutine to clean up idle UDP streams (inactive > 60s)
	stopReaper := make(chan struct{})
	defer close(stopReaper)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopReaper:
				return
			case <-ticker.C:
				now := time.Now().Unix()
				sessionMu.Lock()
				for k, s := range sessions {
					if now-s.lastActive.Load() > 60 || s.closed.Load() {
						s.closed.Store(true)
						_ = s.stream.Close()
						delete(sessions, k)
					}
				}
				sessionMu.Unlock()
			}
		}
	}()

	// Keep reading TCP connection until client closes it (signaling end of UDP association)
	tcpClosed := make(chan struct{})
	go func() {
		defer close(tcpClosed)
		buf := make([]byte, 1)
		for {
			if _, err := tcpConn.Read(buf); err != nil {
				return
			}
		}
	}()

	// UDP packet loop
	packetBuf := make([]byte, 65535)
	for {
		select {
		case <-tcpClosed:
			return
		default:
		}

		_ = udpConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, clientAddr, err := udpConn.ReadFrom(packetBuf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}

		if n < 10 || packetBuf[2] != 0 {
			continue // Drop invalid or fragmented packets
		}

		atyp := packetBuf[3]
		var destHost string
		var destPort uint16
		var headerLen int

		switch atyp {
		case protocol.AtypIPv4:
			if n < 10 {
				continue
			}
			destHost = net.IP(packetBuf[4:8]).String()
			destPort = binary.BigEndian.Uint16(packetBuf[8:10])
			headerLen = 10
		case protocol.AtypDomain:
			dLen := int(packetBuf[4])
			if n < 5+dLen+2 {
				continue
			}
			destHost = string(packetBuf[5 : 5+dLen])
			destPort = binary.BigEndian.Uint16(packetBuf[5+dLen : 5+dLen+2])
			headerLen = 5 + dLen + 2
		case protocol.AtypIPv6:
			if n < 22 {
				continue
			}
			destHost = net.IP(packetBuf[4:20]).String()
			destPort = binary.BigEndian.Uint16(packetBuf[20:22])
			headerLen = 22
		default:
			continue
		}

		destAddr := net.JoinHostPort(destHost, strconv.Itoa(int(destPort)))
		payload := make([]byte, n-headerLen)
		copy(payload, packetBuf[headerLen:n])
		header := make([]byte, headerLen)
		copy(header, packetBuf[:headerLen])

		// Process packet concurrently
		go func(targetAddr net.Addr, dest string, hdr, data []byte) {
			if isLocalHost(dest) {
				localConn, err := net.DialTimeout("udp", dest, 3*time.Second)
				if err == nil {
					defer localConn.Close()
					_, _ = localConn.Write(data)
					respBuf := make([]byte, 65535)
					_ = localConn.SetReadDeadline(time.Now().Add(2 * time.Second))
					rn, rerr := localConn.Read(respBuf)
					if rerr == nil && rn > 0 {
						outPkt := make([]byte, len(hdr)+rn)
						copy(outPkt, hdr)
						copy(outPkt[len(hdr):], respBuf[:rn])
						_, _ = udpConn.WriteTo(outPkt, targetAddr)
					}
				}
				return
			}

			sessionKey := targetAddr.String() + "|" + dest

			sessionMu.Lock()
			s, exists := sessions[sessionKey]
			if exists && s.closed.Load() {
				delete(sessions, sessionKey)
				exists = false
			}

			if !exists {
				sess, err := c.getSession()
				if err != nil {
					sessionMu.Unlock()
					return
				}
				stream, err := sess.OpenStream()
				if err != nil {
					sessionMu.Unlock()
					return
				}

				if err := protocol.WriteTargetAddress(stream, protocol.NetUDP, dest); err != nil {
					stream.Close()
					sessionMu.Unlock()
					return
				}

				s = &clientUDPSession{
					stream: stream,
					header: hdr,
				}
				s.lastActive.Store(time.Now().Unix())
				sessions[sessionKey] = s

				// Launch background reader for UDP responses from VPS
				go func(sessObj *clientUDPSession, key string, client net.Addr) {
					defer func() {
						sessObj.closed.Store(true)
						_ = sessObj.stream.Close()
						sessionMu.Lock()
						delete(sessions, key)
						sessionMu.Unlock()
					}()

					lenBuf := make([]byte, 2)
					for {
						_ = sessObj.stream.SetReadDeadline(time.Now().Add(60 * time.Second))
						if _, err := io.ReadFull(sessObj.stream, lenBuf); err != nil {
							return
						}
						pktLen := int(binary.BigEndian.Uint16(lenBuf))
						if pktLen <= 0 || pktLen > 65535 {
							return
						}
						pkt := make([]byte, pktLen)
						if _, err := io.ReadFull(sessObj.stream, pkt); err != nil {
							return
						}

						sessObj.lastActive.Store(time.Now().Unix())
						c.bytesDown.Add(uint64(pktLen))

						// Prepend SOCKS5 UDP header and send back to client
						respPkt := make([]byte, len(sessObj.header)+pktLen)
						copy(respPkt, sessObj.header)
						copy(respPkt[len(sessObj.header):], pkt)

						// Apply microsecond delay jitter on inbound UDP packets
						if jitter := c.calculateUDPJitter(); jitter > 0 {
							time.Sleep(jitter)
						}

						_, _ = udpConn.WriteTo(respPkt, client)
					}
				}(s, sessionKey, targetAddr)
			}
			sessionMu.Unlock()

			// Apply microsecond delay jitter on outbound UDP packets
			if jitter := c.calculateUDPJitter(); jitter > 0 {
				time.Sleep(jitter)
			}

			// Send packet payload across the multiplexed stream
			s.lastActive.Store(time.Now().Unix())
			s.writeMu.Lock()
			frame := make([]byte, 2+len(data))
			binary.BigEndian.PutUint16(frame[0:2], uint16(len(data)))
			copy(frame[2:], data)
			_ = s.stream.SetWriteDeadline(time.Now().Add(5 * time.Second))
			_, err := s.stream.Write(frame)
			s.writeMu.Unlock()

			if err != nil {
				s.closed.Store(true)
				_ = s.stream.Close()
				sessionMu.Lock()
				delete(sessions, sessionKey)
				sessionMu.Unlock()
			} else {
				c.bytesUp.Add(uint64(len(data)))
			}
		}(clientAddr, destAddr, header, payload)
	}
}

func (c *Client) acceptHttpLoop() {
	for {
		conn, err := c.httpListener.Accept()
		if err != nil {
			return
		}
		go c.handleHttpClient(conn)
	}
}

func (c *Client) handleHttpClient(conn net.Conn) {
	defer conn.Close()
	c.activeConns.Add(1)
	defer c.activeConns.Add(-1)

	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		return
	}

	var destAddr string
	if req.Method == http.MethodConnect {
		destAddr = req.RequestURI
		if !strings.Contains(destAddr, ":") {
			destAddr += ":443"
		}
	} else {
		// Serve PAC script dynamically
		if req.URL != nil && (req.URL.Path == "/pac" || req.URL.Path == "/proxy.pac") {
			c.mu.RLock()
			router := c.router
			c.mu.RUnlock()
			pacScript := ""
			if router != nil {
				pacScript = router.GeneratePAC(c.httpAddr)
			}
			w := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/x-ns-proxy-autoconfig\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(pacScript), pacScript)
			_, _ = conn.Write([]byte(w))
			return
		}

		host := req.URL.Host
		if host == "" {
			host = req.Host
		}
		if !strings.Contains(host, ":") {
			host += ":80"
		}
		destAddr = host
	}

	clientRWC := customReadWriteCloser{
		Reader: reader,
		Writer: conn,
		Closer: conn,
	}

	// Route evaluation
	c.mu.RLock()
	router := c.router
	c.mu.RUnlock()

	var action routing.Action = routing.ActionProxy
	if isLocalHost(destAddr) {
		action = routing.ActionDirect
	} else if router != nil {
		action = router.Match(destAddr)
	}

	if action == routing.ActionBlock {
		_, _ = conn.Write([]byte("HTTP/1.1 403 Forbidden\r\n\r\n"))
		return
	}

	if action == routing.ActionDirect {
		_, portStr, _ := net.SplitHostPort(destAddr)
		if isLocalHost(destAddr) && (portStr == "10809" || portStr == "10808" || portStr == "18080") {
			_, _ = conn.Write([]byte("HTTP/1.1 403 Forbidden\r\n\r\n"))
			return
		}

		localTarget, err := net.DialTimeout("tcp", destAddr, 5*time.Second)
		if err != nil {
			_, _ = conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
			return
		}
		defer localTarget.Close()

		if req.Method == http.MethodConnect {
			_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
			c.pipeBidirectional(clientRWC, localTarget)
			return
		}

		isUpgrade := strings.EqualFold(req.Header.Get("Upgrade"), "websocket") ||
			strings.Contains(strings.ToLower(req.Header.Get("Connection")), "upgrade")

		if !isUpgrade {
			req.Close = true
			req.Header.Set("Connection", "close")
		}

		var reqBuf bytes.Buffer
		if err := req.Write(&reqBuf); err == nil {
			c.bytesUp.Add(uint64(reqBuf.Len()))
			_, _ = localTarget.Write(reqBuf.Bytes())
		}
		c.pipeBidirectional(clientRWC, localTarget)
		return
	}

	if req.Method == http.MethodConnect {
		stream, err := c.openStreamWithRetry(destAddr)
		if err != nil {
			c.Log(fmt.Sprintf("[Error] Tunnel unavailable for %s: %v", destAddr, err))
			_, _ = conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
			return
		}
		defer stream.Close()

		_, err = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		if err != nil {
			return
		}
		c.pipeBidirectional(clientRWC, stream)
		return
	}

	// Plain HTTP to remote target
	stream, err := c.openStreamWithRetry(destAddr)
	if err != nil {
		c.Log(fmt.Sprintf("[Error] Tunnel unavailable for %s: %v", destAddr, err))
		_, _ = conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer stream.Close()

	req.Header.Del("Proxy-Connection")
	req.Header.Del("Proxy-Authenticate")
	req.Header.Del("Proxy-Authorization")

	isUpgrade := strings.EqualFold(req.Header.Get("Upgrade"), "websocket") ||
		strings.Contains(strings.ToLower(req.Header.Get("Connection")), "upgrade")

	if !isUpgrade {
		req.Close = true
		req.Header.Set("Connection", "close")
	}

	var reqBuf bytes.Buffer
	if err := req.Write(&reqBuf); err != nil {
		return
	}
	c.bytesUp.Add(uint64(reqBuf.Len()))
	if _, err := stream.Write(reqBuf.Bytes()); err != nil {
		return
	}
	c.pipeBidirectional(clientRWC, stream)
}

func (c *Client) pipeBidirectional(local io.ReadWriteCloser, remote io.ReadWriteCloser) {
	var once sync.Once
	closeBoth := func() {
		_ = local.Close()
		_ = remote.Close()
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Local -> Remote (Upload)
	go func() {
		defer wg.Done()
		defer once.Do(closeBoth)
		buf := make([]byte, 32768)
		for {
			n, err := local.Read(buf)
			if n > 0 {
				c.bytesUp.Add(uint64(n))
				if _, wErr := remote.Write(buf[:n]); wErr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
	}()

	// Remote -> Local (Download)
	go func() {
		defer wg.Done()
		defer once.Do(closeBoth)
		buf := make([]byte, 32768)
		for {
			n, err := remote.Read(buf)
			if n > 0 {
				c.bytesDown.Add(uint64(n))
				if _, wErr := local.Write(buf[:n]); wErr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
	}()

	wg.Wait()
}

func (c *Client) runSpeedMonitor() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.RLock()
		running := c.running
		c.mu.RUnlock()
		if !running {
			return
		}

		currentUp := c.bytesUp.Load()
		currentDown := c.bytesDown.Load()

		diffUp := currentUp - c.lastBytesUp
		diffDown := currentDown - c.lastBytesDown

		c.lastBytesUp = currentUp
		c.lastBytesDown = currentDown

		c.speedUp.Store(diffUp)
		c.speedDown.Store(diffDown)
	}
}

func (c *Client) runPingMonitor() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.RLock()
		running := c.running
		hm := c.healthMonitor
		addr := c.profile.ServerAddr
		c.mu.RUnlock()

		if !running || addr == "" {
			continue
		}

		if hm != nil {
			// HealthMonitor handles active HTTP 204 URL-testing through the tunnel
			continue
		}

		start := time.Now()
		conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
		if err == nil {
			ms := time.Since(start).Milliseconds()
			c.latency.Store(ms)
			conn.Close()
		}
	}
}

func (c *Client) calculateUDPJitter() time.Duration {
	c.mu.RLock()
	maxJitter := c.udpJitterMax
	enabled := c.udpObfs
	c.mu.RUnlock()

	if !enabled || maxJitter <= 0 {
		return 0
	}

	maxMicros := maxJitter.Microseconds()
	if maxMicros <= 0 {
		return 0
	}

	rnd := protocol.RandomBytes(4)
	val := binary.BigEndian.Uint32(rnd)
	jitterMicros := int64(val) % (maxMicros + 1)
	return time.Duration(jitterMicros) * time.Microsecond
}

// DialTunnel establishes a new multiplexed virtual connection through the active encrypted tunnel
func (c *Client) DialTunnel(destAddr string) (net.Conn, error) {
	c.mu.RLock()
	dialer := c.tunnelDialer
	c.mu.RUnlock()
	if dialer != nil {
		return dialer(destAddr)
	}
	return c.openStreamWithRetry(destAddr)
}

// SetTunnelDialer configures a custom tunnel dialer (useful for unit tests)
func (c *Client) SetTunnelDialer(dialer func(destAddr string) (net.Conn, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tunnelDialer = dialer
}

// GetServers returns a copy of configured server profiles
func (c *Client) GetServers() []ServerProfile {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res := make([]ServerProfile, len(c.servers))
	copy(res, c.servers)
	return res
}

// SetServers updates configured server profiles
func (c *Client) SetServers(servers []ServerProfile) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.servers = make([]ServerProfile, len(servers))
	copy(c.servers, servers)
}

// GetActiveGroupID returns current subscription group ID
func (c *Client) GetActiveGroupID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.activeGroupID
}

// SetActiveGroupID updates active subscription group ID
func (c *Client) SetActiveGroupID(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.activeGroupID = id
}

// GetSNIPoolManager returns the SNI pool manager
func (c *Client) GetSNIPoolManager() *SNIPoolManager {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sniMgr
}

// SetUDPJitterMax configures maximum UDP timing jitter
func (c *Client) SetUDPJitterMax(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.udpJitterMax = d
}

// SetUDPObfuscation toggles UDP obfuscation and jitter
func (c *Client) SetUDPObfuscation(enable bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.udpObfs = enable
}

// StartHealthMonitor attaches and activates health monitoring and auto-failover
func (c *Client) StartHealthMonitor(cfg HealthConfig, onSwitch SwitchCallback) HealthMonitor {
	c.mu.Lock()
	if c.healthMonitor != nil {
		c.healthMonitor.Stop()
	}
	hm := NewHealthMonitor(c, cfg)
	c.healthMonitor = hm
	c.mu.Unlock()

	hm.StartAutoFailover(onSwitch)
	return hm
}

// GetHealthMonitor returns active health monitor if any
func (c *Client) GetHealthMonitor() HealthMonitor {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.healthMonitor
}

// Log adds a message to the internal log buffer
func (c *Client) Log(msg string) {
	entry := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	log.Println(entry)
	c.logMu.Lock()
	if len(c.logs) >= 200 {
		c.logs = c.logs[1:]
	}
	c.logs = append(c.logs, entry)
	c.logMu.Unlock()
}

// GetLogs returns the buffered connection logs
func (c *Client) GetLogs() []string {
	c.logMu.Lock()
	defer c.logMu.Unlock()
	copied := make([]string, len(c.logs))
	copy(copied, c.logs)
	return copied
}

// GetStats returns current live telemetry data
func (c *Client) GetStats() Stats {
	c.mu.RLock()
	running := c.running
	var connTime int64
	if running {
		connTime = c.startTime.Unix()
	}
	c.mu.RUnlock()

	return Stats{
		BytesUp:       c.bytesUp.Load(),
		BytesDown:     c.bytesDown.Load(),
		SpeedUp:       float64(c.speedUp.Load()),
		SpeedDown:     float64(c.speedDown.Load()),
		ActiveConns:   c.activeConns.Load(),
		Connected:     running,
		ConnectedTime: connTime,
		Latency:       int(c.latency.Load()),
	}
}
