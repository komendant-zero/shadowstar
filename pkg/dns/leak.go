package dns

// LeakPreventer manages operating system DNS leak prevention and policy routing.
type LeakPreventer interface {
	Enable(dnsServerIP string) error
	Disable() error
}

// NewLeakPreventer returns an OS-appropriate LeakPreventer implementation.
func NewLeakPreventer() LeakPreventer {
	return newLeakPreventer()
}
