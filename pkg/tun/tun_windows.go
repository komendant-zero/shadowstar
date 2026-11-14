//go:build windows

package tun

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Start initializes the native Wintun adapter, configures routing tables, and starts the packet engine
func (m *Manager) Start(vpsAddr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return nil
	}

	// 1. Resolve remote VPS IP
	vpsHost := vpsAddr
	if host, _, err := net.SplitHostPort(vpsAddr); err == nil {
		vpsHost = host
	}
	vpsHost = strings.TrimSpace(vpsHost)

	// 2. Discover physical default gateway
	defGW := getDefaultGateway()
	if defGW == "" {
		defGW = "192.168.1.1"
	}

	// 3. Create or open native Wintun device
	devName := m.cfg.DeviceName
	if devName == "" {
		devName = "ShadowStar"
	}

	dev, err := CreateWintunDevice(devName)
	if err != nil {
		return fmt.Errorf("failed to initialize Wintun device %q: %w", devName, err)
	}
	m.device = dev

	// 4. Configure adapter IPv4 address and DNS via netsh
	addr := m.cfg.IPv4Address
	if addr == "" {
		addr = "10.255.0.2"
	}
	mask := m.cfg.IPv4Mask
	if mask == "" {
		mask = "255.255.255.0"
	}

	time.Sleep(500 * time.Millisecond) // Allow Windows NDIS stack to recognize adapter
	setIPCmd := fmt.Sprintf(`netsh interface ip set address name="%s" source=static addr=%s mask=%s gateway=none`, devName, addr, mask)
	runHiddenCmd("cmd", "/C", setIPCmd)

	setDNSCmd := fmt.Sprintf(`netsh interface ip set dns name="%s" static 198.18.0.1`, devName)
	runHiddenCmd("cmd", "/C", setDNSCmd)

	// 5. Add persistent direct route to VPS server via physical gateway
	if vpsHost != "" && net.ParseIP(vpsHost) != nil {
		vpsRoute := fmt.Sprintf(`route add %s mask 255.255.255.255 %s metric 1`, vpsHost, defGW)
		runHiddenCmd("cmd", "/C", vpsRoute)
	}

	// 6. Add default routes via TUN (0.0.0.0/1 and 128.0.0.0/1 - standard dual /1 route override)
	gateway := m.cfg.IPv4Gateway
	if gateway == "" {
		gateway = "10.255.0.1"
	}
	route1 := fmt.Sprintf(`route add 0.0.0.0 mask 128.0.0.0 %s metric 5`, gateway)
	route2 := fmt.Sprintf(`route add 128.0.0.0 mask 128.0.0.0 %s metric 5`, gateway)
	runHiddenCmd("cmd", "/C", route1)
	runHiddenCmd("cmd", "/C", route2)

	// 7. Start packet processor netstack
	ns := NewNetStack(dev, m.client, m.router, m.fakeDNS)
	ns.Start()
	m.netstack = ns

	// 8. Kill Switch
	if m.cfg.KillSwitch {
		_ = m.killSwitch.Enable(vpsAddr)
	}

	m.running = true
	return nil
}

// Stop tears down the packet engine, cleans up routing tables, and closes Wintun
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	// 1. Disable Kill Switch
	if m.killSwitch != nil {
		_ = m.killSwitch.Disable()
	}

	// 2. Stop packet engine
	if m.netstack != nil {
		m.netstack.Stop()
		m.netstack = nil
	}

	// 3. Remove routes
	runHiddenCmd("cmd", "/C", `route delete 0.0.0.0 mask 128.0.0.0 >nul 2>&1`)
	runHiddenCmd("cmd", "/C", `route delete 128.0.0.0 mask 128.0.0.0 >nul 2>&1`)

	// 4. Close and release Wintun adapter
	if m.device != nil {
		_ = m.device.Close()
		m.device = nil
	}

	m.running = false
	return nil
}

func getDefaultGateway() string {
	out, err := exec.Command("cmd", "/C", `route print 0.0.0.0`).Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 5 && fields[0] == "0.0.0.0" && fields[1] == "0.0.0.0" {
			gw := fields[2]
			if !strings.HasPrefix(gw, "10.255.") && !strings.HasPrefix(gw, "10.8.") {
				return gw
			}
		}
	}
	return ""
}

func runHiddenCmd(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Run()
}
