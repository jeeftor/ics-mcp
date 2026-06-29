package icsmcp

import (
	"fmt"
	"strings"
	"time"

	"github.com/apognu/gocal"
	"github.com/google/uuid"
)

// ParseICS parses event instances between now and now+lookahead.
func ParseICS(raw string, now time.Time, lookahead time.Duration) ([]EventInstance, error) {
	start := now.UTC().Add(-24 * time.Hour)
	end := now.UTC().Add(lookahead)
	parser := gocal.NewParser(strings.NewReader(raw))
	parser.Start = &start
	parser.End = &end
	if err := parser.Parse(); err != nil {
		return nil, fmt.Errorf("parse ics: %w", err)
	}
	events := make([]EventInstance, 0, len(parser.Events))
	for _, parsed := range parser.Events {
		if parsed.Start == nil || parsed.End == nil {
			continue
		}
		name := parsed.Summary
		if name == "" {
			name = "(untitled)"
		}
		uid := parsed.Uid
		if uid == "" {
			uid = uuid.NewString()
		}
		meetingURL, meetingURLType := ExtractMeetingURL(parsed.URL, parsed.Location, parsed.Description)
		cancelled := strings.EqualFold(parsed.Status, "CANCELLED") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "canceled:") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "cancelled:")
		events = append(events, EventInstance{
			ID:             uuid.NewString(),
			UID:            uid,
			Name:           name,
			Description:    parsed.Description,
			MeetingURL:     meetingURL,
			MeetingURLType: meetingURLType,
			Cancelled:      cancelled,
			AllDay:         parsed.End.Sub(*parsed.Start) >= 24*time.Hour,
			Start:          parsed.Start.UTC(),
			End:            parsed.End.UTC(),
		})
	}
	return events, nil
}
