//go:build !windows

package tray

type Handlers struct {
	OnConnectToggle    func()
	OnSysProxyToggle   func()
	OnTunToggle        func()
	OnKillSwitchToggle func()
	OnOpenWeb          func()
	OnExit             func()

	IsConnected    func() bool
	IsSysProxy     func() bool
	IsTun          func() bool
	IsKillSwitch   func() bool
	GetActiveNode  func() string
}

type TrayApp struct{}

func New(handlers Handlers) *TrayApp {
	return &TrayApp{}
}

func (t *TrayApp) Start() error {
	return nil
}

func (t *TrayApp) UpdateTooltip(tipText string) {}

func (t *TrayApp) Stop() {}

func IsAutoStartEnabled() bool {
	return false
}

func ToggleAutoStart() bool {
	return false
}

func OpenBrowser(url string) {}
