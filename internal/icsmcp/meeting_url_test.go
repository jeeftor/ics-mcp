package icsmcp

import "testing"

func TestExtractMeetingURLPrefersKnownJoinProviders(t *testing.T) {
	gotURL, gotType := ExtractMeetingURL(
		"https://example.invalid/reference",
		"Join https://zoom.us/j/12345 or https://teams.microsoft.com/l/meetup-join/abc123",
	)

	if gotURL != "https://teams.microsoft.com/l/meetup-join/abc123" {
		t.Fatalf("meeting URL = %q", gotURL)
	}
	if gotType != "teams" {
		t.Fatalf("meeting URL type = %q", gotType)
	}
}

func TestExtractMeetingURLCleansTrailingPunctuation(t *testing.T) {
	gotURL, gotType := ExtractMeetingURL("Join here: https://meet.google.com/abc-defg-hij.")

	if gotURL != "https://meet.google.com/abc-defg-hij" {
		t.Fatalf("meeting URL = %q", gotURL)
	}
	if gotType != "meet" {
		t.Fatalf("meeting URL type = %q", gotType)
	}
}

func TestExtractMeetingURLClassifiesWebexAndGenericJoinLinks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		text     string
		wantURL  string
		wantType string
	}{
		{
			name:     "webex",
			text:     "Webex: https://example.webex.com/meet/team-room",
			wantURL:  "https://example.webex.com/meet/team-room",
			wantType: "webex",
		},
		{
			name:     "generic join",
			text:     "Join: https://events.example.test/path/join/abc123",
			wantURL:  "https://events.example.test/path/join/abc123",
			wantType: "link",
		},
		{
			name:     "generic link",
			text:     "Reference: https://example.test/calendar/details",
			wantURL:  "https://example.test/calendar/details",
			wantType: "link",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotURL, gotType := ExtractMeetingURL(tc.text)
			if gotURL != tc.wantURL || gotType != tc.wantType {
				t.Fatalf("ExtractMeetingURL() = (%q, %q), want (%q, %q)", gotURL, gotType, tc.wantURL, tc.wantType)
			}
		})
	}
}

func TestExtractMeetingURLCleansEscapedNewlineMarkersAndSkipsInvalidURLs(t *testing.T) {
	gotURL, gotType := ExtractMeetingURL(
		"broken https:///",
		"Join https://teams.microsoft.com/l/meetup-join/abc123\\nDESCRIPTION:extra",
	)

	if gotURL != "https://teams.microsoft.com/l/meetup-join/abc123" {
		t.Fatalf("meeting URL = %q", gotURL)
	}
	if gotType != "teams" {
		t.Fatalf("meeting URL type = %q", gotType)
	}

	gotURL, gotType = ExtractMeetingURL("no URLs here")
	if gotURL != "" || gotType != "" {
		t.Fatalf("empty extraction = (%q, %q), want empty", gotURL, gotType)
	}
}

func TestExtractMeetingURLRequiresExactProviderHosts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		text     string
		wantType string
	}{
		{name: "teams subdomain", text: "https://teams.microsoft.com/l/meetup-join/x", wantType: "teams"},
		{name: "teams nested subdomain", text: "https://sub.teams.microsoft.com/x", wantType: "teams"},
		{name: "teams suffix attack", text: "https://evilteams.microsoft.com/x", wantType: "link"},
		{name: "teams domain suffix attack", text: "https://teams.microsoft.com.evil.test/x", wantType: "link"},
		{name: "zoom subdomain", text: "https://us02web.zoom.us/j/123", wantType: "zoom"},
		{name: "zoom suffix attack", text: "https://notzoom.us.evil.test/j/1", wantType: "link"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, gotType := ExtractMeetingURL(tc.text)
			if gotType != tc.wantType {
				t.Fatalf("ExtractMeetingURL(%q) type = %q, want %q", tc.text, gotType, tc.wantType)
			}
		})
	}
}

func TestClassifyMeetingURLSkipsMalformedRawURL(t *testing.T) {
	got := classifyMeetingURL("%")
	if got != (meetingURLCandidate{}) {
		t.Fatalf("classifyMeetingURL malformed = %#v, want empty candidate", got)
	}
}
