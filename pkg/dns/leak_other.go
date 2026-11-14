//go:build !windows

package dns

type noopLeakPreventer struct{}

func newLeakPreventer() LeakPreventer {
	return &noopLeakPreventer{}
}

func (n *noopLeakPreventer) Enable(dnsServerIP string) error {
	return nil
}

func (n *noopLeakPreventer) Disable() error {
	return nil
}
