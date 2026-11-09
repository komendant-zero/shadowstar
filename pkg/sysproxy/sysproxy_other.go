//go:build !windows

package sysproxy

// SetSystemProxy stub for non-windows
func SetSystemProxy(proxyServer string) error {
	return nil
}

// ClearSystemProxy stub for non-windows
func ClearSystemProxy() error {
	return nil
}

// GetSystemProxyStatus stub for non-windows
func GetSystemProxyStatus() bool {
	return false
}
