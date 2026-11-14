//go:build windows

package tun

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

type WindowsKillSwitch struct {
	active bool
	mu     sync.Mutex
}

var defaultKillSwitch = &WindowsKillSwitch{}

func GetKillSwitch() *WindowsKillSwitch {
	return defaultKillSwitch
}

// Enable activates Windows Firewall rules blocking all outbound traffic except VPN tunnel and local subnets
func (ks *WindowsKillSwitch) Enable(vpsAddr string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if ks.active {
		return nil
	}

	vpsHost := vpsAddr
	if host, _, err := net.SplitHostPort(vpsAddr); err == nil {
		vpsHost = host
	}
	vpsHost = strings.TrimSpace(vpsHost)

	// Clean any previous residual rule
	_ = ks.disableInternal()

	// Exemptions: Local subnets, loopback, Wintun adapter subnet, and VPS server IP
	exemptions := "127.0.0.1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,169.254.0.0/16,10.255.0.0/24"
	if vpsHost != "" && net.ParseIP(vpsHost) != nil {
		exemptions += "," + vpsHost
	}

	// PowerShell command creating Windows Firewall outbound block rule with exemptions
	psCmd := fmt.Sprintf(`New-NetFirewallRule -DisplayName "ShadowStar-KillSwitch" -Direction Outbound -Action Block -RemoteAddress "!%s" -Enabled True -Profile Any -ErrorAction Stop`, exemptions)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if err := cmd.Run(); err != nil {
		// Fallback to netsh if NetSecurity cmdlets aren't installed or fail
		netshCmd := fmt.Sprintf(`netsh advfirewall firewall add rule name="ShadowStar-KillSwitch" dir=out action=block remoteip="!%s" enable=yes`, exemptions)
		fbCmd := exec.Command("cmd", "/C", netshCmd)
		fbCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if fbErr := fbCmd.Run(); fbErr != nil {
			return fmt.Errorf("failed to enable Kill Switch firewall rule: %v (fallback: %v)", err, fbErr)
		}
	}

	ks.active = true
	return nil
}

// Disable removes the Windows Firewall rule
func (ks *WindowsKillSwitch) Disable() error {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return ks.disableInternal()
}

func (ks *WindowsKillSwitch) disableInternal() error {
	psCmd := `Remove-NetFirewallRule -DisplayName "ShadowStar-KillSwitch" -ErrorAction SilentlyContinue; netsh advfirewall firewall delete rule name="ShadowStar-KillSwitch" >nul 2>&1`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Run()
	ks.active = false
	return nil
}

func (ks *WindowsKillSwitch) IsActive() bool {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return ks.active
}
