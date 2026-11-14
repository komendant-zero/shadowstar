//go:build !windows

package tun

type WindowsKillSwitch struct{}

func GetKillSwitch() *WindowsKillSwitch {
	return &WindowsKillSwitch{}
}

func (ks *WindowsKillSwitch) Enable(vpsAddr string) error {
	return nil
}

func (ks *WindowsKillSwitch) Disable() error {
	return nil
}

func (ks *WindowsKillSwitch) IsActive() bool {
	return false
}
