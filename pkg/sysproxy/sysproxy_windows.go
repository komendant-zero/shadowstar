//go:build windows

package sysproxy

import (
	"fmt"
	"os/exec"
	"syscall"
)

var (
	modwininet            = syscall.NewLazyDLL("wininet.dll")
	procInternetSetOption = modwininet.NewProc("InternetSetOptionW")
)

const (
	INTERNET_OPTION_SETTINGS_CHANGED = 39
	INTERNET_OPTION_REFRESH          = 37
)

func notifyWinINet() {
	procInternetSetOption.Call(0, uintptr(INTERNET_OPTION_SETTINGS_CHANGED), 0, 0)
	procInternetSetOption.Call(0, uintptr(INTERNET_OPTION_REFRESH), 0, 0)
}

// SetSystemProxy configures the Windows internet proxy
func SetSystemProxy(proxyServer string) error {
	cmd := exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "1", "/f")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable proxy: %w", err)
	}

	cmd2 := exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyServer", "/t", "REG_SZ", "/d", proxyServer, "/f")
	cmd2.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd2.Run(); err != nil {
		return fmt.Errorf("failed to set proxy server: %w", err)
	}

	// Bypass local addresses, internal loopback, and private subnets with wildcards
	override := "<local>;localhost*;127.*;::1;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*"
	cmd3 := exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyOverride", "/t", "REG_SZ", "/d", override, "/f")
	cmd3.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd3.Run()

	// Sync DefaultConnectionSettings and SavedLegacySettings in WinINet
	psScript := fmt.Sprintf(`$connPath = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings\Connections'
$proxy = '%s'
$bypass = '%s'
$pBytes = [System.Text.Encoding]::ASCII.GetBytes($proxy)
$bBytes = [System.Text.Encoding]::ASCII.GetBytes($bypass)
$blob = [byte[]]::new(16 + $pBytes.Length + 4 + $bBytes.Length + 32)
$blob[0] = 70
$blob[8] = 3
[BitConverter]::GetBytes([int]$pBytes.Length).CopyTo($blob, 12)
[Array]::Copy($pBytes, 0, $blob, 16, $pBytes.Length)
$off = 16 + $pBytes.Length
[BitConverter]::GetBytes([int]$bBytes.Length).CopyTo($blob, $off)
[Array]::Copy($bBytes, 0, $blob, $off + 4, $bBytes.Length)
foreach ($val in @('DefaultConnectionSettings', 'SavedLegacySettings')) {
    Set-ItemProperty -Path $connPath -Name $val -Value $blob -Type Binary -ErrorAction SilentlyContinue
}`, proxyServer, override)

	psCmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	psCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = psCmd.Run()

	notifyWinINet()
	return nil
}

// ClearSystemProxy disables the Windows internet proxy and cleans all connection overrides
func ClearSystemProxy() error {
	cmd := exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "0", "/f")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Run()

	cmdDel := exec.Command("reg", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyServer", "/f")
	cmdDel.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmdDel.Run()

	// Clear DefaultConnectionSettings and SavedLegacySettings via powershell
	psScript := `$connPath = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings\Connections'
foreach ($val in @('DefaultConnectionSettings', 'SavedLegacySettings')) {
    $item = Get-ItemProperty -Path $connPath -Name $val -ErrorAction SilentlyContinue
    if ($item) {
        $b = $item.$val
        if ($b -and $b.Length -gt 8) {
            $b[8] = [byte]1
            Set-ItemProperty -Path $connPath -Name $val -Value $b -Type Binary -ErrorAction SilentlyContinue
        }
    }
}`
	psCmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	psCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = psCmd.Run()

	notifyWinINet()
	return nil
}

// GetSystemProxyStatus checks whether the system proxy is currently enabled
func GetSystemProxyStatus() bool {
	out, err := exec.Command("reg", "query", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyEnable").Output()
	if err != nil {
		return false
	}
	return string(out) != "" && (containsHex1(out))
}

func containsHex1(b []byte) bool {
	s := string(b)
	return len(s) > 0 && (contains(s, "0x1"))
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
