package icsmcp

import (
	"fmt"
	"html"
	"net/url"
	"strings"
)

const (
	MeetingFormatJSON               = "json"
	MeetingFormatText               = "text"
	MeetingFormatTelegramText       = "tg-text"
	MeetingFormatTelegramHTML       = "tg-html"
	MeetingFormatTelegramMarkdownV2 = "tg-markdownv2"
)

// NormalizeMeetingFormat resolves user-facing aliases to canonical format names.
func NormalizeMeetingFormat(format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", MeetingFormatJSON:
		return MeetingFormatJSON, nil
	case MeetingFormatText, MeetingFormatTelegramText, "telegram":
		return MeetingFormatTelegramText, nil
	case MeetingFormatTelegramHTML, "telegram-html":
		return MeetingFormatTelegramHTML, nil
	case MeetingFormatTelegramMarkdownV2, "telegram-markdownv2", "markdownv2":
		return MeetingFormatTelegramMarkdownV2, nil
	default:
		return "", fmt.Errorf("unsupported meeting format %q", format)
	}
}

func isJSONMeetingFormat(format string) bool {
	normalized, err := NormalizeMeetingFormat(format)
	return err == nil && normalized == MeetingFormatJSON
}

// FormatMeetings renders meetings as text for paste-friendly or Telegram delivery.
func FormatMeetings(meetings []Meeting, format string) (string, error) {
	normalized, err := NormalizeMeetingFormat(format)
	if err != nil {
		return "", err
	}
	if normalized == MeetingFormatJSON {
		return "", nil
	}
	return formatMeetingLines(meetings, normalized), nil
}

// FormatGroupedMeetings renders grouped meetings as text for paste-friendly or Telegram delivery.
func FormatGroupedMeetings(groups []CalendarMeetingGroup, format string) (string, error) {
	normalized, err := NormalizeMeetingFormat(format)
	if err != nil {
		return "", err
	}
	if normalized == MeetingFormatJSON {
		return "", nil
	}
	sections := make([]string, 0, len(groups))
	for _, group := range groups {
		lines := []string{formatHeader(group.CalendarName, normalized)}
		body := formatMeetingLines(group.Meetings, normalized)
		if body != "" {
			lines = append(lines, body)
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}
	return strings.Join(sections, "\n\n"), nil
}

// FormatBusyBlocks renders availability blocks without meeting titles or details.
func FormatBusyBlocks(busy []BusyBlock, format string) (string, error) {
	normalized, err := NormalizeMeetingFormat(format)
	if err != nil {
		return "", err
	}
	if normalized == MeetingFormatJSON {
		return "", nil
	}
	lines := []string{formatHeader("Busy", normalized)}
	for _, block := range busy {
		lines = append(lines, formatBusyLine(block, normalized))
	}
	return strings.Join(lines, "\n"), nil
}

func formatMeetingLines(meetings []Meeting, format string) string {
	lines := []string{}
	currentHeader := ""
	for _, meeting := range meetings {
		header := meetingDayHeader(meeting)
		if header != "" && header != currentHeader {
			lines = append(lines, formatHeader(header, format))
			currentHeader = header
		}
		lines = append(lines, formatMeetingLine(meeting, format))
		if meeting.MeetingURL != "" {
			lines = append(lines, formatJoinLine(meeting.MeetingURL, format))
		}
	}
	return strings.Join(lines, "\n")
}

func formatBusyLine(block BusyBlock, format string) string {
	parts := []string{block.When}
	if block.Duration != "" {
		parts[0] += " (" + block.Duration + ")"
	}
	if block.Calendar != "" {
		parts = append(parts, block.Calendar)
	}
	if block.Ongoing {
		parts = append(parts, "ongoing")
	}
	if block.AllDay {
		parts = append(parts, "all day")
	}
	line := strings.Join(parts, " - ")
	switch format {
	case MeetingFormatTelegramHTML:
		return "- " + html.EscapeString(line)
	case MeetingFormatTelegramMarkdownV2:
		return `\- ` + escapeTelegramMarkdownV2(line)
	default:
		return "- " + line
	}
}

func meetingDayHeader(meeting Meeting) string {
	date := meeting.Date
	if parsed, err := parseMeetingDate(meeting.Date); err == nil {
		date = parsed
	}
	return strings.TrimSpace(strings.Join([]string{meeting.Day, date}, " "))
}

func parseMeetingDate(date string) (string, error) {
	parts := strings.Split(date, "-")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid date %q", date)
	}
	monthNames := map[string]string{
		"01": "Jan", "02": "Feb", "03": "Mar", "04": "Apr",
		"05": "May", "06": "Jun", "07": "Jul", "08": "Aug",
		"09": "Sep", "10": "Oct", "11": "Nov", "12": "Dec",
	}
	month, ok := monthNames[parts[1]]
	if !ok {
		return "", fmt.Errorf("invalid date %q", date)
	}
	day := strings.TrimLeft(parts[2], "0")
	if day == "" {
		day = "0"
	}
	return month + " " + day, nil
}

func formatHeader(value string, format string) string {
	switch format {
	case MeetingFormatTelegramHTML:
		return "<b>" + html.EscapeString(value) + "</b>"
	case MeetingFormatTelegramMarkdownV2:
		return "*" + escapeTelegramMarkdownV2(value) + "*"
	default:
		return value
	}
}

func formatMeetingLine(meeting Meeting, format string) string {
	timeRange := compactTimeRange(meeting.Start, meeting.End)
	title := strings.TrimSpace(meeting.Name)
	if title == "" {
		title = meeting.Title
	}
	switch format {
	case MeetingFormatTelegramHTML:
		return "- " + html.EscapeString(timeRange) + " " + "<b>" + html.EscapeString(title) + "</b>"
	case MeetingFormatTelegramMarkdownV2:
		return `\- ` + escapeTelegramMarkdownV2(timeRange) + " " + "*" + escapeTelegramMarkdownV2(title) + "*"
	default:
		return "- " + timeRange + " " + title
	}
}

func formatJoinLine(joinURL string, format string) string {
	switch format {
	case MeetingFormatTelegramHTML:
		return `  <a href="` + html.EscapeString(joinURL) + `">Join</a>`
	case MeetingFormatTelegramMarkdownV2:
		return "  [Join](" + escapeTelegramMarkdownV2URL(joinURL) + ")"
	default:
		return "  Join: " + joinURL
	}
}

func escapeTelegramMarkdownV2(value string) string {
	var b strings.Builder
	for _, r := range value {
		if strings.ContainsRune(`_*[]()~`+"`"+`>#+-=|{}.!`, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func escapeTelegramMarkdownV2URL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.String() == "" {
		return escapeTelegramMarkdownV2(value)
	}
	replacer := strings.NewReplacer(`\`, `\\`, `)`, `\)`)
	return replacer.Replace(value)
}
