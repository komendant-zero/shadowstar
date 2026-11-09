//go:build !windows

package nativegui

import (
	"errors"

	"github.com/shadowstar/shadowstar/pkg/client"
)

// Run is a stub for non-Windows platforms
func Run(cfgPath string, c *client.Client, cfg *client.Config, autoConnect bool) error {
	return errors.New("native GUI is only supported on Windows")
}
