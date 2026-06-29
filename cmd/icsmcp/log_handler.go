package icsmcp

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

type colorSlogHandler struct {
	w        io.Writer
	level    slog.Level
	attrs    []slog.Attr
	groups   []string
	renderer *lipgloss.Renderer
	color    bool
	mu       *sync.Mutex
}

func newColorSlogHandler(w io.Writer, level slog.Level) slog.Handler {
	return newSlogHandler(w, level, true)
}

func newPlainSlogHandler(w io.Writer, level slog.Level) slog.Handler {
	return newSlogHandler(w, level, false)
}

func newSlogHandler(w io.Writer, level slog.Level, color bool) slog.Handler {
	renderer := lipgloss.NewRenderer(w)
	renderer.SetColorProfile(termenv.ANSI256)
	return &colorSlogHandler{
		w:        w,
		level:    level,
		renderer: renderer,
		color:    color,
		mu:       &sync.Mutex{},
	}
}

func (h *colorSlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *colorSlogHandler) Handle(_ context.Context, record slog.Record) error {
	var b strings.Builder
	timestamp := record.Time
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	h.writeKeyValue(&b, "time", timestamp.Format(time.RFC3339), logValueTimestamp)
	b.WriteByte(' ')
	b.WriteString(h.styleKey("level"))
	b.WriteByte('=')
	b.WriteString(h.colorLevel(record.Level))
	b.WriteByte(' ')
	h.writeKeyValue(&b, "msg", strconv.Quote(record.Message), logValueMessage)
	h.writeAttrs(&b, h.attrs)
	record.Attrs(func(attr slog.Attr) bool {
		h.writeAttr(&b, attr)
		return true
	})
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *colorSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(slicesClone(h.attrs), attrs...)
	return &next
}

func (h *colorSlogHandler) WithGroup(name string) slog.Handler {
	next := *h
	if name != "" {
		next.groups = append(slicesClone(h.groups), name)
	}
	return &next
}

func (h *colorSlogHandler) writeAttrs(b *strings.Builder, attrs []slog.Attr) {
	for _, attr := range attrs {
		h.writeAttr(b, attr)
	}
}

func (h *colorSlogHandler) writeAttr(b *strings.Builder, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}
	b.WriteByte(' ')
	if len(h.groups) > 0 {
		b.WriteString(h.styleKey(strings.Join(h.groups, ".")))
		b.WriteByte('.')
	}
	h.writeKeyValue(b, attr.Key, formatLogValue(attr.Value), logValueDefault)
}

func (h *colorSlogHandler) colorLevel(level slog.Level) string {
	label := level.String()
	if !h.color {
		return label
	}
	switch {
	case level <= slog.LevelDebug:
		return h.renderer.NewStyle().Foreground(lipgloss.Color("63")).Render(label)
	case level < slog.LevelWarn:
		return h.renderer.NewStyle().Foreground(lipgloss.Color("42")).Render(label)
	case level < slog.LevelError:
		return h.renderer.NewStyle().Foreground(lipgloss.Color("214")).Render(label)
	default:
		return h.renderer.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(label)
	}
}

func (h *colorSlogHandler) writeKeyValue(b *strings.Builder, key string, value string, kind logValueKind) {
	b.WriteString(h.styleKey(key))
	b.WriteByte('=')
	b.WriteString(h.styleValue(value, kind))
}

func (h *colorSlogHandler) styleKey(key string) string {
	if !h.color {
		return key
	}
	return h.renderer.NewStyle().Foreground(lipgloss.Color("75")).Render(key)
}

func (h *colorSlogHandler) styleValue(value string, kind logValueKind) string {
	if !h.color {
		return value
	}
	switch kind {
	case logValueTimestamp:
		return h.renderer.NewStyle().Foreground(lipgloss.Color("244")).Render(value)
	case logValueMessage:
		return h.renderer.NewStyle().Foreground(lipgloss.Color("229")).Bold(true).Render(value)
	default:
		return h.renderer.NewStyle().Foreground(lipgloss.Color("159")).Render(value)
	}
}

type logValueKind int

const (
	logValueDefault logValueKind = iota
	logValueTimestamp
	logValueMessage
)

func formatLogValue(value slog.Value) string {
	switch value.Kind() {
	case slog.KindString:
		raw := value.String()
		if raw == "" || strings.ContainsAny(raw, " \t\n\r\"=") {
			return strconv.Quote(raw)
		}
		return raw
	case slog.KindTime:
		return value.Time().Format(time.RFC3339)
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindGroup:
		attrs := value.Group()
		parts := make([]string, 0, len(attrs))
		for _, attr := range attrs {
			parts = append(parts, attr.Key+"="+formatLogValue(attr.Value.Resolve()))
		}
		return "{" + strings.Join(parts, " ") + "}"
	default:
		return fmt.Sprint(value.Any())
	}
}

func slicesClone[S ~[]E, E any](value S) S {
	if value == nil {
		return nil
	}
	return append(S{}, value...)
}
