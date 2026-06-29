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
