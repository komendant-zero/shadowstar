package client

import (
	"errors"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	// SNIQuarantineDuration defines how long a failed SNI is kept in quarantine (5 minutes)
	SNIQuarantineDuration = 300 * time.Second
	// MaxConsecutiveSNIErrors is the consecutive error threshold before an SNI is quarantined
	MaxConsecutiveSNIErrors = 2
)

// SNIPoolManager manages dynamic round-robin rotation, quarantine, and fallback across alternate SNI domains
type SNIPoolManager struct {
	mu                 sync.Mutex
	pool               []string
	cursor             int
	quarantine         map[string]time.Time
	failureCount       map[string]int
	quarantineDuration time.Duration
}

// NewSNIPoolManager creates a new SNIPoolManager initialized with candidate SNIs and active SNI
func NewSNIPoolManager(pool []string, activeSNI string) *SNIPoolManager {
	mgr := &SNIPoolManager{
		quarantine:         make(map[string]time.Time),
		failureCount:       make(map[string]int),
		quarantineDuration: SNIQuarantineDuration,
	}
	mgr.SetPool(pool, activeSNI)
	return mgr
}

// SetPool updates candidate pool and positions cursor at activeSNI
func (m *SNIPoolManager) SetPool(pool []string, activeSNI string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var clean []string
	seen := make(map[string]bool)
	for _, s := range pool {
		s = sanitizeSNI(s)
		if s != "" && !seen[s] {
			clean = append(clean, s)
			seen[s] = true
		}
	}

	activeSNI = sanitizeSNI(activeSNI)
	if activeSNI != "" && !seen[activeSNI] {
		clean = append([]string{activeSNI}, clean...)
		seen[activeSNI] = true
	}

	if len(clean) == 0 {
		if activeSNI != "" {
			clean = []string{activeSNI}
		} else {
			clean = []string{"www.microsoft.com"}
		}
	}

	m.pool = clean
	m.cursor = 0
	for i, s := range m.pool {
		if s == activeSNI {
			m.cursor = i
			break
		}
	}
}

// GetPool returns a copy of current SNI pool candidates
func (m *SNIPoolManager) GetPool() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	res := make([]string, len(m.pool))
	copy(res, m.pool)
	return res
}

// SetQuarantineDuration allows setting custom quarantine duration (useful for unit testing)
func (m *SNIPoolManager) SetQuarantineDuration(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.quarantineDuration = d
}

// SelectSNI selects the active non-quarantined SNI.
// It maintains stability (stickiness) and does NOT cycle domains upon each connection.
// If the current SNI is quarantined, it switches to the next available healthy candidate.
// If all candidates are currently quarantined, falls back to the earliest-expiring SNI to prevent deadlock.
func (m *SNIPoolManager) SelectSNI() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.pool) == 0 {
		return "www.microsoft.com"
	}

	now := time.Now()
	// 1. If current cursor is healthy and not quarantined, stick with it!
	curIdx := m.cursor % len(m.pool)
	curCand := m.pool[curIdx]
	if expiry, ok := m.quarantine[curCand]; !ok || now.After(expiry) {
		m.cursor = curIdx
		return curCand
	}

	// 2. Current candidate is quarantined -> find the first healthy alternative starting from cursor+1
	for i := 1; i < len(m.pool); i++ {
		idx := (curIdx + i) % len(m.pool)
		cand := m.pool[idx]
		if expiry, ok := m.quarantine[cand]; !ok || now.After(expiry) {
			m.cursor = idx
			return cand
		}
	}

	// 3. All candidates quarantined -> pick earliest expiring candidate to avoid permanent connection blackout
	var earliestSNI string
	var earliestTime time.Time
	earliestIdx := 0
	for i, cand := range m.pool {
		expiry := m.quarantine[cand]
		if earliestSNI == "" || expiry.Before(earliestTime) {
			earliestSNI = cand
			earliestTime = expiry
			earliestIdx = i
		}
	}
	m.cursor = earliestIdx
	return earliestSNI
}

// NextSNI explicitly advances to the next candidate in the pool (e.g., when forced rotation is requested).
func (m *SNIPoolManager) NextSNI() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.pool) == 0 {
		return "www.microsoft.com"
	}
	m.cursor = (m.cursor + 1) % len(m.pool)
	return m.pool[m.cursor]
}

// ReportFailure records a connection or handshake failure for an SNI.
// TCP RST errors and timeouts immediately trigger the quarantine.
// Other errors trigger quarantine once consecutive failures reach MaxConsecutiveSNIErrors.
func (m *SNIPoolManager) ReportFailure(sni string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sni = sanitizeSNI(sni)
	m.failureCount[sni]++
	isRST := isTCPRSTError(err)
	isTO := isTimeoutError(err)

	duration := m.quarantineDuration
	if duration <= 0 {
		duration = SNIQuarantineDuration
	}

	if isRST || isTO || m.failureCount[sni] >= MaxConsecutiveSNIErrors {
		m.quarantine[sni] = time.Now().Add(duration)
	}

	// Advance cursor away from the failing SNI so next retry tries an alternate candidate
	if len(m.pool) > 1 {
		curIdx := m.cursor % len(m.pool)
		if m.pool[curIdx] == sni {
			m.cursor = (curIdx + 1) % len(m.pool)
		}
	}
}

// ReportSuccess resets failure counters and lifts any quarantine for an SNI.
// It also locks in the successful SNI as the active cursor so subsequent connections stick to it.
func (m *SNIPoolManager) ReportSuccess(sni string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sni = sanitizeSNI(sni)
	m.failureCount[sni] = 0
	delete(m.quarantine, sni)

	for i, s := range m.pool {
		if s == sni {
			m.cursor = i
			break
		}
	}
}

// IsQuarantined checks whether an SNI is currently in quarantine
func (m *SNIPoolManager) IsQuarantined(sni string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	sni = sanitizeSNI(sni)
	expiry, ok := m.quarantine[sni]
	return ok && time.Now().Before(expiry)
}

// GetQuarantinedSNIs returns a snapshot map of all currently quarantined SNIs and their expiration times
func (m *SNIPoolManager) GetQuarantinedSNIs() map[string]time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	res := make(map[string]time.Time)
	for k, v := range m.quarantine {
		if now.Before(v) {
			res[k] = v
		}
	}
	return res
}

func isTCPRSTError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "connection reset") ||
		strings.Contains(s, "reset by peer") ||
		strings.Contains(s, "wsaeconnreset") ||
		strings.Contains(s, "econnreset")
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "timeout") ||
		strings.Contains(s, "deadline exceeded")
}
