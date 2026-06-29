package icsmcp

import (
	"net/url"
	"regexp"
	"strings"
)

var httpURLPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

type meetingURLCandidate struct {
	url      string
	urlType  string
	priority int
}

// ExtractMeetingURL returns the best online-meeting URL found in event text.
func ExtractMeetingURL(values ...string) (string, string) {
	var best meetingURLCandidate
	for _, value := range values {
		for _, raw := range httpURLPattern.FindAllString(value, -1) {
			candidate := classifyMeetingURL(cleanMeetingURL(raw))
			if candidate.url == "" {
				continue
			}
			if best.url == "" || candidate.priority < best.priority {
				best = candidate
			}
		}
	}
	return best.url, best.urlType
}

func cleanMeetingURL(raw string) string {
	raw = strings.TrimSpace(raw)
	for _, marker := range []string{`\\n`, `\n`, `\\N`, `\N`} {
		if before, _, ok := strings.Cut(raw, marker); ok {
			raw = before
		}
	}
	raw = strings.TrimRight(raw, ".,);]")
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.String()
}

func classifyMeetingURL(raw string) meetingURLCandidate {
	if raw == "" {
		return meetingURLCandidate{}
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return meetingURLCandidate{}
	}
	host := strings.ToLower(parsed.Host)
	path := strings.ToLower(parsed.Path)
	switch {
	case strings.Contains(host, "teams.microsoft.com"):
		return meetingURLCandidate{url: raw, urlType: "teams", priority: 1}
	case strings.Contains(host, "zoom.us"):
		return meetingURLCandidate{url: raw, urlType: "zoom", priority: 2}
	case strings.Contains(host, "meet.google.com"):
		return meetingURLCandidate{url: raw, urlType: "meet", priority: 3}
	case strings.Contains(host, "webex.com"):
		return meetingURLCandidate{url: raw, urlType: "webex", priority: 4}
	case strings.Contains(path, "meetup-join") || strings.Contains(path, "join"):
		return meetingURLCandidate{url: raw, urlType: "link", priority: 5}
	default:
		return meetingURLCandidate{url: raw, urlType: "link", priority: 10}
	}
}
