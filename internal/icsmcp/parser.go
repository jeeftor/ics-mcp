package icsmcp

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/apognu/gocal"
	"github.com/google/uuid"
)

var tzidParamPattern = regexp.MustCompile(`TZID=("[^"]+"|[^;:\r\n]+)`)

var windowsTimezoneMap = map[string]string{
	"Central Standard Time":      "America/Chicago",
	"Eastern Standard Time":      "America/New_York",
	"GMT Standard Time":          "Europe/London",
	"Mountain Standard Time":     "America/Denver",
	"Pacific Standard Time":      "America/Los_Angeles",
	"South Africa Standard Time": "Africa/Johannesburg",
	"UTC":                        "UTC",
	"US Mountain Standard Time":  "America/Phoenix",
	"W. Europe Standard Time":    "Europe/Berlin",
}

func init() {
	gocal.SetTZMapper(func(tzid string) (*time.Location, error) {
		location, _, err := loadLocation(tzid)
		if err != nil {
			return nil, errors.New("unmapped timezone")
		}
		return location, nil
	})
}

func loadLocation(value string) (*time.Location, string, error) {
	value = strings.TrimSpace(value)
	if mapped, ok := windowsTimezoneMap[value]; ok {
		location, err := time.LoadLocation(mapped)
		return location, mapped, err
	}
	location, err := time.LoadLocation(value)
	return location, value, err
}

// ParseICS parses event instances between now and now+lookahead.
func ParseICS(raw string, now time.Time, lookahead time.Duration) ([]EventInstance, error) {
	start := now.UTC().Add(-24 * time.Hour)
	end := now.UTC().Add(lookahead)
	if err := validateICSTimezones(raw); err != nil {
		return nil, err
	}
	parser := gocal.NewParser(strings.NewReader(raw))
	parser.Start = &start
	parser.End = &end
	if err := parser.Parse(); err != nil {
		return nil, fmt.Errorf("parse ics: %w", err)
	}
	events := make([]EventInstance, 0, len(parser.Events))
	for _, parsed := range parser.Events {
		event, ok := normalizeParsedEvent(parsed)
		if !ok {
			continue
		}
		events = append(events, event)
	}
	return events, nil
}

func normalizeParsedEvent(parsed gocal.Event) (EventInstance, bool) {
	if parsed.Start == nil || parsed.End == nil {
		return EventInstance{}, false
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
	return EventInstance{
		ID:             uuid.NewString(),
		UID:            uid,
		Name:           name,
		Description:    parsed.Description,
		MeetingURL:     meetingURL,
		MeetingURLType: meetingURLType,
		Cancelled:      cancelled,
		AllDay:         parsed.End.Sub(*parsed.Start) >= 24*time.Hour,
		Recurring:      parsed.IsRecurring || parsed.RecurrenceID != "",
		RecurrenceID:   parsed.RecurrenceID,
		Start:          parsed.Start.UTC(),
		End:            parsed.End.UTC(),
	}, true
}

func validateICSTimezones(raw string) error {
	seen := map[string]bool{}
	for _, match := range tzidParamPattern.FindAllStringSubmatch(raw, -1) {
		tzid := strings.Trim(match[1], `"`)
		if seen[tzid] {
			continue
		}
		seen[tzid] = true
		if _, _, err := loadLocation(tzid); err != nil {
			return fmt.Errorf("parse ics timezone %q: %w", tzid, err)
		}
	}
	return nil
}
