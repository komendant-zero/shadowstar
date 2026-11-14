//go:build !windows

package tun

import (
	"errors"
)

// Start stub for non-Windows platforms
func (m *Manager) Start(vpsAddr string) error {
	return errors.New("native TUN is currently supported on Windows and Android")
}

// Stop stub for non-Windows platforms
func (m *Manager) Stop() error {
	return nil
}
