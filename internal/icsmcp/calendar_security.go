package icsmcp

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// calendarRequest validates a feed URL before allowing the server to fetch it.
func (s *Service) calendarRequest(ctx context.Context, rawURL string) (*http.Request, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("parse calendar URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("calendar URL must use http or https")
	}
	if parsed.Hostname() == "" {
		return nil, fmt.Errorf("calendar URL must include a host")
	}
	if !s.allowPrivateCalendarHosts && privateCalendarHost(ctx, parsed.Hostname()) {
		return nil, fmt.Errorf("calendar URL resolves to a private or local address")
	}
	return http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
}

func privateCalendarHost(ctx context.Context, host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return privateCalendarIP(ip)
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return true
	}
	for _, address := range addresses {
		if privateCalendarIP(address.IP) {
			return true
		}
	}
	return false
}

func privateCalendarIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func (s *Service) readCalendarBody(body io.Reader) ([]byte, error) {
	limited := io.LimitReader(body, s.maxCalendarBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > s.maxCalendarBytes {
		return nil, fmt.Errorf("calendar response exceeds %d byte limit", s.maxCalendarBytes)
	}
	return data, nil
}
