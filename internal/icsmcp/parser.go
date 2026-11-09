package icsmcp

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	duration "github.com/ChannelMeter/iso8601duration"
	ical "github.com/arran4/golang-ical"
	"github.com/google/uuid"
	"github.com/teambition/rrule-go"
)

var tzidParamPattern = regexp.MustCompile(`TZID=("[^"]+"|[^;:\r\n]+)`)

// maxRecurrenceGenerations bounds how many occurrences a single RRULE may
// generate while expanding one feed. It protects the refresher from
// pathological high-frequency rules anchored far in the past.
const maxRecurrenceGenerations = 500_000

// parseBackfillWindow is how far before the refresh instant ParseICS retains
// event instances. It covers consumers that render recently-past days (the
// household dashboard fetches a 7-day lookback for its dimmed past events).
const parseBackfillWindow = 7 * 24 * time.Hour

var windowsTimezoneMap = map[string]string{
	"Central Standard Time":      "America/Chicago",
	"Eastern Standard Time":      "America/New_York",
	"GMT Standard Time":          "Europe/London",
	"Mountain Standard Time":     "America/Denver",
	"Pacific Standard Time":      "America/Los_Angeles",
	"South Africa Standard Time": "Africa/Johannesburg",
	"UTC":                        "UTC",
	"US Mountain Standard Time":  "America/Phoenix",
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

// ParseICS parses event instances between now-parseBackfillWindow and now+lookahead.
func ParseICS(raw string, now time.Time, lookahead time.Duration) ([]EventInstance, error) {
	windowStart := now.UTC().Add(-parseBackfillWindow)
	windowEnd := now.UTC().Add(lookahead)
	if err := validateICSTimezones(raw); err != nil {
		return nil, err
	}
	cal, err := ical.ParseCalendarWithOptions(strings.NewReader(raw), ical.WithWindowsTimezoneMapping())
	if err != nil {
		return nil, fmt.Errorf("parse ics: %w", err)
	}
	vevents := cal.Events()
	overrides := recurrenceOverrides(vevents)
	events := make([]EventInstance, 0, len(vevents))
	for _, ve := range vevents {
		instances, err := expandEvent(ve, overrides, windowStart, windowEnd)
		if err != nil {
			return nil, err
		}
		events = append(events, instances...)
	}
	return events, nil
}

// recurrenceOverrides maps each recurring series UID to the UTC instants of
// RECURRENCE-ID exception events so generated occurrences can be suppressed.
func recurrenceOverrides(vevents []*ical.VEvent) map[string]map[int64]bool {
	overrides := map[string]map[int64]bool{}
	for _, ve := range vevents {
		if ve.GetProperty(ical.ComponentPropertyRrule) != nil {
			continue
		}
		ridProp := ve.GetProperty(ical.ComponentPropertyRecurrenceId)
		if ridProp == nil {
			continue
		}
		rid, err := ve.GetRecurrenceID()
		if err != nil {
			continue
		}
		if isAllDayValue(ve.GetProperty(ical.ComponentPropertyDtStart)) {
			rid = utcDateMidnight(rid)
		}
		uid := propertyValue(ve, ical.ComponentPropertyUniqueId)
		if overrides[uid] == nil {
			overrides[uid] = map[int64]bool{}
		}
		overrides[uid][rid.UTC().Unix()] = true
	}
	return overrides
}

func expandEvent(ve *ical.VEvent, overrides map[string]map[int64]bool, windowStart, windowEnd time.Time) ([]EventInstance, error) {
	uid := propertyValue(ve, ical.ComponentPropertyUniqueId)
	if uid == "" {
		return nil, fmt.Errorf("could not parse event without UID")
	}
	startProp := ve.GetProperty(ical.ComponentPropertyDtStart)
	if startProp == nil {
		return nil, fmt.Errorf("could not parse event without DTSTART")
	}
	startAllDay := isAllDayValue(startProp)
	var start time.Time
	var err error
	if startAllDay {
		var t time.Time
		t, err = ve.GetAllDayStartAt()
		start = utcDateMidnight(t)
	} else {
		start, err = ve.GetStartAt()
	}
	if err != nil {
		return nil, fmt.Errorf("parse event start: %w", err)
	}
	end, ok, err := eventEnd(ve, start, startAllDay)
	if err != nil {
		return nil, fmt.Errorf("parse event end: %w", err)
	}
	if !ok {
		return nil, nil
	}

	name := propertyValue(ve, ical.ComponentPropertySummary)
	if name == "" {
		name = "(untitled)"
	}
	description := propertyValue(ve, ical.ComponentPropertyDescription)
	location := propertyValue(ve, ical.ComponentPropertyLocation)
	url := propertyValue(ve, ical.ComponentPropertyUrl)
	status := propertyValue(ve, ical.ComponentPropertyStatus)
	meetingURL, meetingURLType := ExtractMeetingURL(url, location, description)
	cancelled := strings.EqualFold(status, "CANCELLED") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "canceled:") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "cancelled:")

	recurrenceID := ""
	if ridProp := ve.GetProperty(ical.ComponentPropertyRecurrenceId); ridProp != nil {
		recurrenceID = ridProp.Value
	}
	rruleProps := ve.GetProperties(ical.ComponentPropertyRrule)
	exdates, err := ve.GetExDates()
	if err != nil {
		return nil, fmt.Errorf("parse event exdates: %w", err)
	}
	rdates, err := ve.GetRDates()
	if err != nil {
		return nil, fmt.Errorf("parse event rdates: %w", err)
	}
	if startAllDay {
		exdates = utcMidnights(exdates)
		rdates = utcMidnights(rdates)
	}
	duration := end.Sub(start)
	excluded := func(t time.Time) bool {
		for _, exdate := range exdates {
			if exdate.Equal(t) {
				return true
			}
		}
		return false
	}

	newInstance := func(start, end time.Time, recurring bool, recurrenceID string) EventInstance {
		// gocal canonicalized inclusive all-day DTEND representations (23:59:59.999)
		// to the exclusive midnight boundary; keep that normalization for parity.
		if start.Hour() == 0 && start.Minute() == 0 && start.Second() == 0 && start.Nanosecond() == 0 &&
			end.Hour() == 23 && end.Minute() == 59 && end.Second() == 59 && end.Nanosecond() == 999000000 {
			end = end.Add(time.Millisecond)
		}
		allDay := end.Sub(start) >= 24*time.Hour
		return EventInstance{
			ID:               uuid.NewString(),
			UID:              uid,
			Name:             name,
			Description:      description,
			MeetingURL:       meetingURL,
			MeetingURLType:   meetingURLType,
			AttendanceStatus: unambiguousAttendanceStatus(attendeeStatuses(ve)),
			Cancelled:        cancelled,
			AllDay:           allDay,
			Recurring:        recurring,
			RecurrenceID:     recurrenceID,
			Start:            start.UTC(),
			End:              end.UTC(),
		}
	}

	if len(rruleProps) == 0 {
		// RFC 5545 allows RDATE without RRULE: DTSTART plus each RDATE forms
		// the recurrence set, with EXDATE and RECURRENCE-ID exclusions applied.
		candidates := append([]time.Time{start}, rdates...)
		seen := map[int64]bool{}
		var out []EventInstance
		for _, occurrence := range candidates {
			key := occurrence.UTC().Unix()
			// RECURRENCE-ID override events share their master's UID, so the
			// override map contains their own instant; they must still emit.
			if seen[key] || excluded(occurrence) || (recurrenceID == "" && overrides[uid][key]) {
				continue
			}
			seen[key] = true
			occurrenceEnd := occurrence.Add(duration)
			if !overlapsWindow(occurrence, occurrenceEnd, windowStart, windowEnd) {
				continue
			}
			out = append(out, newInstance(occurrence, occurrenceEnd, len(rdates) > 0 || recurrenceID != "", recurrenceID))
		}
		return out, nil
	}

	var out []EventInstance
	for _, rp := range rruleProps {
		rule, err := rrule.StrToRRule(strings.TrimSpace(rp.Value))
		if err != nil {
			return nil, fmt.Errorf("parse ics rrule %q: %w", rp.Value, err)
		}
		set := &rrule.Set{}
		set.DTStart(start)
		set.RRule(rule)
		if len(exdates) > 0 {
			set.SetExDates(exdates)
		}
		if len(rdates) > 0 {
			set.SetRDates(rdates)
		}
		// Iterate manually instead of Set.Between so expansion can be bounded:
		// the iterator always starts at DTSTART, so a high-frequency rule
		// anchored years in the past would otherwise generate millions of
		// occurrences on every refresh.
		next := set.Iterator()
		generated := 0
		for {
			occurrence, ok := next()
			if !ok || occurrence.After(windowEnd) {
				break
			}
			generated++
			if generated > maxRecurrenceGenerations {
				return nil, fmt.Errorf("parse ics rrule %q: recurrence expansion exceeded %d occurrences", rp.Value, maxRecurrenceGenerations)
			}
			occurrenceEnd := occurrence.Add(duration)
			if !occurrenceEnd.After(windowStart) || !occurrence.Before(windowEnd) {
				continue
			}
			if overrides[uid][occurrence.UTC().Unix()] {
				continue
			}
			out = append(out, newInstance(occurrence, occurrenceEnd, true, ""))
		}
	}
	return out, nil
}

func overlapsWindow(start, end, windowStart, windowEnd time.Time) bool {
	return start.Before(windowEnd) && end.After(windowStart)
}

func eventEnd(ve *ical.VEvent, start time.Time, startAllDay bool) (time.Time, bool, error) {
	if endProp := ve.GetProperty(ical.ComponentPropertyDtEnd); endProp != nil {
		var end time.Time
		var err error
		if isAllDayValue(endProp) {
			var t time.Time
			t, err = ve.GetAllDayEndAt()
			end = utcDateMidnight(t)
		} else {
			end, err = ve.GetEndAt()
		}
		if err != nil {
			return time.Time{}, false, err
		}
		return end, true, nil
	}
	if durationProp := ve.GetProperty(ical.ComponentPropertyDuration); durationProp != nil {
		d, err := duration.FromString(durationProp.Value)
		if err != nil {
			return time.Time{}, false, err
		}
		return start.Add(d.ToDuration()), true, nil
	}
	if startAllDay {
		return start.Add(24 * time.Hour), true, nil
	}
	return time.Time{}, false, nil
}

func propertyValue(ve *ical.VEvent, name ical.ComponentProperty) string {
	prop := ve.GetProperty(name)
	if prop == nil {
		return ""
	}
	return prop.Value
}

func propertyParam(prop *ical.IANAProperty, name string) string {
	for key, values := range prop.ICalParameters {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func isAllDayValue(prop *ical.IANAProperty) bool {
	return prop != nil && strings.EqualFold(propertyParam(prop, "VALUE"), "DATE")
}

func utcDateMidnight(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func utcMidnights(times []time.Time) []time.Time {
	out := make([]time.Time, 0, len(times))
	for _, t := range times {
		out = append(out, utcDateMidnight(t))
	}
	return out
}

func attendeeStatuses(ve *ical.VEvent) []string {
	props := ve.GetProperties(ical.ComponentPropertyAttendee)
	statuses := make([]string, 0, len(props))
	for _, prop := range props {
		statuses = append(statuses, propertyParam(prop, "PARTSTAT"))
	}
	return statuses
}

// unambiguousAttendanceStatus returns a response only when an ICS event has one
// attendee. Multi-attendee events do not identify the calendar owner, so their
// individual response states must not be guessed or exposed.
func unambiguousAttendanceStatus(statuses []string) string {
	if len(statuses) != 1 {
		return ""
	}
	switch strings.ToUpper(strings.TrimSpace(statuses[0])) {
	case "ACCEPTED":
		return "accepted"
	case "TENTATIVE":
		return "tentative"
	case "DECLINED":
		return "declined"
	case "NEEDS-ACTION", "NEEDS_ACTION":
		return "needs-action"
	default:
		return ""
	}
}

func validateICSTimezones(raw string) error {
	seen := map[string]bool{}
	for _, match := range tzidParamPattern.FindAllStringSubmatch(raw, -1) {
		tzid := strings.Trim(match[1], `"`)
		if seen[tzid] {
			continue
		}
		seen[tzid] = true
		if ical.WindowsTimezoneToIANA(tzid) != nil {
			continue
		}
		if _, err := time.LoadLocation(tzid); err != nil {
			return fmt.Errorf("parse ics timezone %q: %w", tzid, err)
		}
	}
	return nil
}
