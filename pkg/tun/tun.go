package tun

import (
	"io"
	"sync"

	"github.com/shadowstar/shadowstar/pkg/client"
	"github.com/shadowstar/shadowstar/pkg/dns"
	"github.com/shadowstar/shadowstar/pkg/routing"
)

// Device represents an OS-level TUN virtual interface
type Device interface {
	io.ReadWriteCloser
	Name() string
}

// Manager controls the L3 TUN virtual adapter, routing and packet engine
type Manager struct {
	cfg        Config
	client     *client.Client
	router     routing.Engine
	fakeDNS    dns.FakeDNS
	device     Device
	netstack   *NetStack
	killSwitch *WindowsKillSwitch
	running    bool
	mu         sync.Mutex
}

// NewManager creates a new TUN Manager
func NewManager(cfg Config, c *client.Client, r routing.Engine, f dns.FakeDNS) *Manager {
	return &Manager{
		cfg:        cfg,
		client:     c,
		router:     r,
		fakeDNS:    f,
		killSwitch: GetKillSwitch(),
	}
}

// IsRunning returns whether L3 TUN is currently active
func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// GetConfig returns the active TUN configuration
func (m *Manager) GetConfig() Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// SetConfig updates the TUN configuration
func (m *Manager) SetConfig(cfg Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
}
