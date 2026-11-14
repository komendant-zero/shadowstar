package dns

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Resolver resolves domains to IP addresses using encrypted protocols (DoH and DoT).
type Resolver interface {
	Resolve(domain string) ([]net.IP, error)
}

type resolverImpl struct {
	cfg        *Config
	httpClient *http.Client
}

// NewResolver creates an encrypted DNS resolver conforming to the specified configuration.
func NewResolver(cfg *Config) (Resolver, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	transport := &http.Transport{
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
	}
	if cfg.TLSConfig != nil {
		transport.TLSClientConfig = cfg.TLSConfig.Clone()
	}
	return &resolverImpl{
		cfg: cfg,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   5 * time.Second,
		},
	}, nil
}

// Resolve queries configured encrypted upstreams sequentially with failover.
func (r *resolverImpl) Resolve(domain string) ([]net.IP, error) {
	domain = normalizeDomainName(domain)
	if domain == "" {
		return nil, errors.New("empty domain")
	}

	// DoH query with ID=0 per RFC 8484
	query := BuildQuery(domain, TypeA, 0)

	var lastErr error

	// 1. Try DoH upstreams
	for _, dohURL := range r.cfg.UpstreamDoH {
		ips, err := r.resolveDoH(dohURL, query)
		if err == nil && len(ips) > 0 {
			return ips, nil
		}
		if err != nil {
			lastErr = err
		}
	}

	// 2. Fall back to DoT upstreams
	for _, dotAddr := range r.cfg.UpstreamDoT {
		ips, err := r.resolveDoT(dotAddr, query)
		if err == nil && len(ips) > 0 {
			return ips, nil
		}
		if err != nil {
			lastErr = err
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("encrypted dns resolution failed: %w", lastErr)
	}
	return nil, errors.New("no encrypted dns upstreams configured or reachable")
}

func (r *resolverImpl) resolveDoH(url string, query []byte) ([]net.IP, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Try POST first
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(query))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		// Fallback to GET with unpadded base64url query parameter
		encoded := base64.RawURLEncoding.EncodeToString(query)
		sep := "?"
		if strings.Contains(url, "?") {
			sep = "&"
		}
		getURL := url + sep + "dns=" + encoded
		getReq, getErr := http.NewRequestWithContext(ctx, http.MethodGet, getURL, nil)
		if getErr != nil {
			return nil, err
		}
		getReq.Header.Set("Accept", "application/dns-message")
		resp, err = r.httpClient.Do(getReq)
		if err != nil {
			return nil, err
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("doh upstream returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	msg, err := ParseMessage(data)
	if err != nil {
		return nil, err
	}

	if msg.RCode != RCodeNoError {
		return nil, fmt.Errorf("dns error rcode %d", msg.RCode)
	}

	var ips []net.IP
	for _, ans := range msg.Answers {
		if ans.Type == TypeA && len(ans.Data) == 4 {
			ips = append(ips, net.IPv4(ans.Data[0], ans.Data[1], ans.Data[2], ans.Data[3]))
		}
	}
	return ips, nil
}

func (r *resolverImpl) resolveDoT(dotAddr string, query []byte) ([]net.IP, error) {
	host, _, err := net.SplitHostPort(dotAddr)
	if err != nil {
		host = dotAddr
		dotAddr = net.JoinHostPort(dotAddr, "853")
	}

	var tlsCfg *tls.Config
	if r.cfg != nil && r.cfg.TLSConfig != nil {
		tlsCfg = r.cfg.TLSConfig.Clone()
	} else {
		tlsCfg = &tls.Config{}
	}
	if tlsCfg.ServerName == "" {
		tlsCfg.ServerName = host
	}

	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 3 * time.Second}, "tcp", dotAddr, tlsCfg)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	// RFC 7858 2-byte big-endian length prefix
	prefix := make([]byte, 2)
	binary.BigEndian.PutUint16(prefix, uint16(len(query)))
	if _, err := conn.Write(prefix); err != nil {
		return nil, err
	}
	if _, err := conn.Write(query); err != nil {
		return nil, err
	}

	if _, err := io.ReadFull(conn, prefix); err != nil {
		return nil, err
	}
	respLen := binary.BigEndian.Uint16(prefix)
	respData := make([]byte, respLen)
	if _, err := io.ReadFull(conn, respData); err != nil {
		return nil, err
	}

	msg, err := ParseMessage(respData)
	if err != nil {
		return nil, err
	}

	if msg.RCode != RCodeNoError {
		return nil, fmt.Errorf("dns error rcode %d", msg.RCode)
	}

	var ips []net.IP
	for _, ans := range msg.Answers {
		if ans.Type == TypeA && len(ans.Data) == 4 {
			ips = append(ips, net.IPv4(ans.Data[0], ans.Data[1], ans.Data[2], ans.Data[3]))
		}
	}
	return ips, nil
}
