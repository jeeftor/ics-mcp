package icsmcp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLLMActionLogsRedactInquiryData(t *testing.T) {
	const question = "Private question that must not be logged"
	const calendarTitle = "Confidential calendar title"
	const token = "private-bearer-token"
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"answer\":\"OK\",\"evidence\":[]}"}}]}`))
	}))
	defer provider.Close()

	svc := newTestService(t)
	var logs bytes.Buffer
	svc.logger = slog.New(slog.NewTextHandler(&logs, nil))
	ctx := context.Background()
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "private", URL: "https://example.test/private.ics"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.store.replaceEvents(ctx, cal.ID, []EventInstance{{ID: "private", UID: "private", Name: calendarTitle, Start: time.Now().UTC(), End: time.Now().UTC().Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: ptr(true), Endpoint: provider.URL, Model: "test-model", APIKey: token}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PreviewInsight(ctx, RunInsightInput{Question: question, DateScope: InsightDateScopeToday}); err != nil {
		t.Fatal(err)
	}
	got := logs.String()
	for _, secret := range []string{provider.URL, token, question, calendarTitle} {
		if strings.Contains(got, secret) {
			t.Fatalf("LLM action logs exposed %q: %s", secret, got)
		}
	}
	for _, want := range []string{"msg=\"llm action started\"", "msg=\"llm action completed\"", "action=inquiry_preview", "event_count=1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("LLM action logs missing %q: %s", want, got)
		}
	}
}

func TestInsightInquiriesPersistScopeAndStarterTemplatesAreOptional(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	enabled := false
	got, err := svc.SaveInsightInquiry(ctx, "school_today", SaveInsightInquiryInput{
		Question: "Do the kids have school today?", CalendarIDs: []string{"school"}, Tags: []string{"School"}, Trigger: InsightTriggerScheduled, Schedule: "06:00", Enabled: &enabled,
	})
	if err != nil || got.Enabled || got.Trigger != InsightTriggerScheduled || got.Schedule != "06:00" || got.DateScope != InsightDateScopeToday || len(got.CalendarIDs) != 1 || len(got.Tags) != 1 {
		t.Fatalf("SaveInsightInquiry = %#v, %v", got, err)
	}
	starter, err := svc.GetInsightInquiry(ctx, "daily_briefing")
	if err != nil || !starter.Builtin || starter.Enabled || starter.DateScope != InsightDateScopeToday {
		t.Fatalf("daily briefing starter = %#v, %v", starter, err)
	}
	if _, err := svc.GetInsightInquiry(ctx, "weekly_outlook"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("weekly outlook should not be a starter inquiry: %v", err)
	}
	if err := svc.DeleteInsightInquiry(ctx, "daily_briefing"); err != nil {
		t.Fatalf("DeleteInsightInquiry(starter) = %v", err)
	}
	if _, err := svc.GetInsightInquiry(ctx, "school_today"); err != nil {
		t.Fatalf("GetInsightInquiry = %v", err)
	}
}

func TestInsightDateScopeFiltersCalendarDataBeforeLLMRequest(t *testing.T) {
	var payload string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		payload = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"answer\":\"OK\",\"evidence\":[]}"}}]}`))
	}))
	defer provider.Close()

	svc := newTestService(t)
	svc.SetClock(func() time.Time { return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC) })
	ctx := context.Background()
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.store.replaceEvents(ctx, cal.ID, []EventInstance{
		{ID: "today", UID: "today", Name: "Today only", Description: "private description", MeetingURL: "https://secret.example", Start: time.Date(2026, 7, 30, 15, 0, 0, 0, time.UTC), End: time.Date(2026, 7, 30, 16, 0, 0, 0, time.UTC)},
		{ID: "tomorrow", UID: "tomorrow", Name: "Tomorrow only", Start: time.Date(2026, 7, 31, 15, 0, 0, 0, time.UTC), End: time.Date(2026, 7, 31, 16, 0, 0, 0, time.UTC)},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: ptr(true), Endpoint: provider.URL, Model: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PreviewInsight(ctx, RunInsightInput{Question: "What is today?", DateScope: InsightDateScopeToday}); err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"Tomorrow only", "private description", "secret.example"} {
		if strings.Contains(payload, absent) {
			t.Fatalf("LLM payload unexpectedly included %q: %s", absent, payload)
		}
	}
	if !strings.Contains(payload, "Today only") {
		t.Fatalf("LLM payload omitted today event: %s", payload)
	}
}

func TestInsightCustomDateScopeValidation(t *testing.T) {
	for _, input := range []SaveInsightInquiryInput{
		{Question: "Q", DateScope: InsightDateScopeCustom, StartDate: "2026-07-31", EndDate: "2026-07-30"},
		{Question: "Q", DateScope: InsightDateScopeCustom, StartDate: "2026-07-01", EndDate: "2026-08-03"},
	} {
		if _, err := newTestService(t).SaveInsightInquiry(context.Background(), "custom", input); err == nil {
			t.Fatalf("SaveInsightInquiry(%#v) error = nil", input)
		}
	}
}

func TestRunInsightPersistsFailureAndUsesNamedInquiry(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: ptr(true), Endpoint: "http://127.0.0.1:1", Model: "test", APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := svc.SaveInsightInquiry(ctx, "daily_briefing", SaveInsightInquiryInput{Question: "What should I know today?", Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunInsight(ctx, RunInsightInput{Name: "daily_briefing"}); err == nil {
		t.Fatal("RunInsight error = nil, want failed call")
	}
	cached, err := svc.GetInsight(ctx, "daily_briefing")
	if err != nil || cached.Error == "" || cached.Question == "" {
		t.Fatalf("failed cached insight = %#v, %v", cached, err)
	}
	if !errors.Is(svc.DeleteInsightInquiry(ctx, "missing"), sql.ErrNoRows) {
		t.Fatal("missing inquiry should return sql.ErrNoRows")
	}
}

func TestInsightsAreDisabledAndProfileIsRedacted(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	if _, err := svc.RunInsight(ctx, RunInsightInput{Name: "daily", Question: "What is today?"}); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("RunInsight disabled error = %v", err)
	}
	profile, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{APIKey: "secret-key", Endpoint: "https://example.invalid", Model: "test"})
	if err != nil || !profile.APIKeyConfigured || strings.Contains(profile.Endpoint, "secret-key") {
		t.Fatalf("redacted profile = %#v, %v", profile, err)
	}
}

func TestLLMEnabledStateCanBeToggledUnlessEnvironmentControlled(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	enabled := true
	profile, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: &enabled})
	if err != nil || !profile.Enabled || profile.Source != "database" {
		t.Fatalf("enabled profile = %#v, %v", profile, err)
	}
	disabled := false
	profile, err = svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: &disabled})
	if err != nil || profile.Enabled {
		t.Fatalf("disabled profile = %#v, %v", profile, err)
	}
	t.Setenv("ICSMCP_LLM_ENABLED", "true")
	if _, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: &disabled}); err == nil || !strings.Contains(err.Error(), "overridden by environment") {
		t.Fatalf("environment-controlled enabled state error = %v", err)
	}
}

func TestLLMConnectionStagingUsesUnsavedValuesWithoutLeakingTheBearerKey(t *testing.T) {
	const bearer = "staged-secret-key"
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+bearer {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"alpha"},{"id":"beta"},{"id":"alpha"},{"id":""}]}`))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	svc := newTestService(t)
	ctx := context.Background()
	input := LLMConnectionInput{Endpoint: provider.URL + "/v1/chat/completions", APIKey: bearer}
	if err := svc.TestLLMEndpoint(ctx, input); err != nil {
		t.Fatalf("TestLLMEndpoint = %v", err)
	}
	models, err := svc.DiscoverLLMModels(ctx, input)
	if err != nil || len(models) != 2 || models[0] != "alpha" || models[1] != "beta" {
		t.Fatalf("DiscoverLLMModels = %#v, %v", models, err)
	}
	if err := svc.TestLLMModel(ctx, LLMModelTestInput{LLMConnectionInput: input, Model: "beta"}); err != nil {
		t.Fatalf("TestLLMModel = %v", err)
	}
	profile, err := svc.LLMProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(profile)
	if strings.Contains(string(encoded), bearer) || profile.Endpoint != "" || profile.Model != "" {
		t.Fatalf("staged test persisted or leaked secrets: %s", encoded)
	}
}

func TestLLMTransportTimeoutsAreSafeAndActionable(t *testing.T) {
	const endpoint = "http://192.168.1.91:13305/v1"
	const apiKey = "private-bearer-key"
	const question = "Private calendar question"
	svc := newTestService(t)
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("Post %q: context deadline exceeded (Client.Timeout exceeded while awaiting headers)", endpoint+"/chat/completions?key="+apiKey)
	})}

	assertSafeTimeout := func(err error) {
		t.Helper()
		if err == nil {
			t.Fatal("LLM request error = nil, want timeout")
		}
		got := err.Error()
		if !strings.Contains(got, "did not respond before the request timed out") || !strings.Contains(got, "no response headers were received") {
			t.Fatalf("timeout error = %q, want actionable no-header message", got)
		}
		for _, privateValue := range []string{endpoint, apiKey, question, "context deadline exceeded"} {
			if strings.Contains(got, privateValue) {
				t.Fatalf("timeout error leaked %q: %q", privateValue, got)
			}
		}
	}

	assertSafeTimeout(svc.TestLLMEndpoint(context.Background(), LLMConnectionInput{Endpoint: endpoint, APIKey: apiKey}))
	assertSafeTimeout(svc.TestLLMModel(context.Background(), LLMModelTestInput{LLMConnectionInput: LLMConnectionInput{Endpoint: endpoint, APIKey: apiKey}, Model: "test"}))
	if _, err := svc.UpdateLLMProfile(context.Background(), UpdateLLMProfileInput{Enabled: ptr(true), Endpoint: endpoint, Model: "test", APIKey: apiKey}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.PreviewInsight(context.Background(), RunInsightInput{Question: question})
	assertSafeTimeout(err)
}

func TestOllamaProfileUsesTagsAndChatEndpoints(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"llama3.2"}]}`))
		case "/api/chat":
			_, _ = w.Write([]byte(`{"message":{"content":"OK"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	svc := newTestService(t)
	input := LLMConnectionInput{Backend: LLMBackendOllama, Endpoint: provider.URL}
	models, err := svc.DiscoverLLMModels(context.Background(), input)
	if err != nil || len(models) != 1 || models[0] != "llama3.2" {
		t.Fatalf("DiscoverLLMModels Ollama = %#v, %v", models, err)
	}
	if err := svc.TestLLMModel(context.Background(), LLMModelTestInput{LLMConnectionInput: input, Model: "llama3.2"}); err != nil {
		t.Fatalf("TestLLMModel Ollama = %v", err)
	}
}

func TestModelDiscoveryExplainsHTMLResponse(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>not an API</html>"))
	}))
	defer provider.Close()

	svc := newTestService(t)
	_, err := svc.DiscoverLLMModels(context.Background(), LLMConnectionInput{Endpoint: provider.URL + "/v1"})
	if err == nil || !strings.Contains(err.Error(), "returned HTML") || strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("DiscoverLLMModels HTML error = %v", err)
	}
}

func TestLemonadeOriginUsesOpenAICompatibleV1Routes(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"lemonade-model"}]}`))
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	svc := newTestService(t)
	input := LLMConnectionInput{Backend: LLMBackendLemonade, Endpoint: provider.URL}
	models, err := svc.DiscoverLLMModels(context.Background(), input)
	if err != nil || len(models) != 1 || models[0] != "lemonade-model" {
		t.Fatalf("DiscoverLLMModels Lemonade = %#v, %v", models, err)
	}
	if err := svc.TestLLMModel(context.Background(), LLMModelTestInput{LLMConnectionInput: input, Model: "lemonade-model"}); err != nil {
		t.Fatalf("TestLLMModel Lemonade = %v", err)
	}
}

func TestLemonadeModelLifecycleLoadsAndWaitsForTheSelectedModel(t *testing.T) {
	var loaded bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/health":
			if loaded {
				_, _ = w.Write([]byte(`{"all_models_loaded":[{"model_name":"lemonade-model"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"all_models_loaded":[]}`))
		case "/v1/load":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["model_name"] != "lemonade-model" || body["pinned"] != true {
				t.Errorf("load body = %#v", body)
			}
			loaded = true
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	svc := newTestService(t)
	svc.lemonadePollInterval = time.Millisecond
	input := LLMModelTestInput{LLMConnectionInput: LLMConnectionInput{Backend: LLMBackendLemonade, Endpoint: provider.URL}, Model: "lemonade-model"}
	if got, err := svc.LemonadeModelStatus(context.Background(), input); err != nil || got.State != LemonadeModelStateAbsent {
		t.Fatalf("LemonadeModelStatus = %#v, %v", got, err)
	}
	got, err := svc.LoadLemonadeModel(context.Background(), input)
	if err != nil || got.State != LemonadeModelStateReady {
		t.Fatalf("LoadLemonadeModel = %#v, %v", got, err)
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), provider.URL) || strings.Contains(string(encoded), "lemonade-model") {
		t.Fatalf("lifecycle response leaked connection details: %s", encoded)
	}
}

func TestRunInsightCachesStructuredAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"answer\":\"No school\",\"evidence\":[\"calendar event\"],\"caveat\":\"Check the feed\"}"}}]}`))
	}))
	defer server.Close()
	svc := newTestService(t)
	ctx := context.Background()
	_, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: ptr(true), Endpoint: server.URL, Model: "test", APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.RunInsight(ctx, RunInsightInput{Name: "school", Question: "Do the kids have school today?"})
	if err != nil || got.Answer != "No school" || len(got.Evidence) != 1 {
		t.Fatalf("RunInsight = %#v, %v", got, err)
	}
	cached, err := svc.GetInsight(ctx, "school")
	if err != nil || cached.SourceHash == "" {
		t.Fatalf("GetInsight = %#v, %v", cached, err)
	}
}

func TestInsightTriggersUseLocalDailyTimeAndOnlyRerunOnChangedData(t *testing.T) {
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"answer\":\"OK\",\"evidence\":[]}"}}]}`))
	}))
	defer provider.Close()

	svc := newTestService(t)
	ctx := context.Background()
	_, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: ptr(true), Endpoint: provider.URL, Model: "test", APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		name     string
		trigger  InsightTrigger
		schedule string
	}{
		{name: "manual", trigger: InsightTriggerManual},
		{name: "daily", trigger: InsightTriggerScheduled, schedule: "06:00"},
		{name: "changes", trigger: InsightTriggerOnChange},
	} {
		if _, err := svc.SaveInsightInquiry(ctx, input.name, SaveInsightInquiryInput{Question: "What changed?", Trigger: input.trigger, Schedule: input.schedule}); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Date(2026, time.July, 30, 5, 59, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	svc.RunDueInsights(ctx)
	if got := calls.Load(); got != 1 { // on_change has no prior cached source.
		t.Fatalf("calls before daily time = %d, want 1", got)
	}
	now = now.Add(time.Minute)
	svc.RunDueInsights(ctx)
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls at daily time = %d, want 2", got)
	}
	svc.RunDueInsights(ctx)
	if got := calls.Load(); got != 2 {
		t.Fatalf("unchanged data reran automatic insight: calls = %d", got)
	}
	now = now.Add(24 * time.Hour)
	svc.RunDueInsights(ctx)
	if got := calls.Load(); got != 3 {
		t.Fatalf("daily insight did not run on next local day: calls = %d", got)
	}
	if _, err := svc.store.db.ExecContext(ctx, `UPDATE insights SET source_hash = 'old' WHERE name = 'changes'`); err != nil {
		t.Fatal(err)
	}
	svc.RunDueInsights(ctx)
	if got := calls.Load(); got != 4 {
		t.Fatalf("changed scoped source did not rerun: calls = %d", got)
	}
}

func TestScheduledInsightRequiresDailyClockTime(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.SaveInsightInquiry(context.Background(), "bad", SaveInsightInquiryInput{Question: "Q", Trigger: InsightTriggerScheduled, Schedule: "24h"})
	if err == nil || !strings.Contains(err.Error(), "HH:MM") {
		t.Fatalf("SaveInsightInquiry invalid daily time error = %v", err)
	}
}

func TestRepeatedInsightUsesValidatedInterval(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	item, err := svc.SaveInsightInquiry(ctx, "frequent", SaveInsightInquiryInput{Question: "What changed?", Trigger: InsightTriggerScheduled, ScheduleMode: InsightScheduleModeRepeat, RepeatInterval: "15m"})
	if err != nil || item.ScheduleMode != InsightScheduleModeRepeat || item.RepeatInterval != "15m" || item.Schedule != "" {
		t.Fatalf("SaveInsightInquiry repeat = %#v, %v", item, err)
	}
	now := time.Date(2026, time.July, 30, 10, 0, 0, 0, time.UTC)
	if !svc.insightScheduledDue(item, now) {
		t.Fatal("new repeated inquiry should be due")
	}
	item.LastRunAt = now.Add(-14 * time.Minute)
	if svc.insightScheduledDue(item, now) {
		t.Fatal("repeated inquiry became due early")
	}
	item.LastRunAt = now.Add(-15 * time.Minute)
	if !svc.insightScheduledDue(item, now) {
		t.Fatal("repeated inquiry was not due at its interval")
	}
	_, err = svc.SaveInsightInquiry(ctx, "bad-repeat", SaveInsightInquiryInput{Question: "Q", Trigger: InsightTriggerScheduled, ScheduleMode: InsightScheduleModeRepeat, RepeatInterval: "10s"})
	if err == nil || !strings.Contains(err.Error(), "repeat interval") {
		t.Fatalf("SaveInsightInquiry invalid repeat interval error = %v", err)
	}
}

func TestPreviewInsightDoesNotPersistOutputOrHistory(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"answer\":\"Preview\",\"evidence\":[]}"}}]}`))
	}))
	defer provider.Close()
	svc := newTestService(t)
	ctx := context.Background()
	if _, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: ptr(true), Endpoint: provider.URL, Model: "test"}); err != nil {
		t.Fatal(err)
	}
	preview, err := svc.PreviewInsight(ctx, RunInsightInput{Question: "What should I know?"})
	if err != nil || preview.Answer != "Preview" {
		t.Fatalf("PreviewInsight = %#v, %v", preview, err)
	}
	if _, err := svc.GetInsight(ctx, "preview"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("preview cached an insight: %v", err)
	}
	if history, err := svc.ListPromptHistory(ctx, "preview", 10); !errors.Is(err, sql.ErrNoRows) || len(history) != 0 {
		t.Fatalf("preview cached run history: %#v, %v", history, err)
	}
}
