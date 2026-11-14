//go:build windows

package dns

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

type windowsLeakPreventer struct {
	ruleAdded bool
}

func newLeakPreventer() LeakPreventer {
	return &windowsLeakPreventer{}
}

func (w *windowsLeakPreventer) Enable(dnsServerIP string) error {
	dnsServerIP = strings.TrimSpace(dnsServerIP)
	if dnsServerIP == "" {
		dnsServerIP = "198.18.0.1"
	}

	// Add NRPT rule for root namespace "." redirecting to designated local resolver
	psCmd := fmt.Sprintf(`Get-DnsClientNrptRule | Where-Object { $_.Comment -eq "ShadowStar" } | Remove-DnsClientNrptRule -Force -ErrorAction SilentlyContinue; Add-DnsClientNrptRule -Namespace "." -Nameservers "%s" -Comment "ShadowStar"`, dnsServerIP)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add NRPT leak prevention rule: %w", err)
	}
	w.ruleAdded = true
	return nil
}

func (w *windowsLeakPreventer) Disable() error {
	if !w.ruleAdded {
		return nil
	}
	psCmd := `Get-DnsClientNrptRule | Where-Object { $_.Comment -eq "ShadowStar" } | Remove-DnsClientNrptRule -Force -ErrorAction SilentlyContinue`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Run()
	w.ruleAdded = false
	return nil
}
