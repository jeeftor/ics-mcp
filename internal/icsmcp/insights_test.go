package icsmcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestInsightInquiriesPersistScopeAndProtectBuiltins(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	enabled := false
	got, err := svc.SaveInsightInquiry(ctx, "school_today", SaveInsightInquiryInput{
		Question: "Do the kids have school today?", CalendarIDs: []string{"school"}, Tags: []string{"School"}, Trigger: InsightTriggerScheduled, Schedule: "06:00", Enabled: &enabled,
	})
	if err != nil || got.Enabled || got.Trigger != InsightTriggerScheduled || got.Schedule != "06:00" || len(got.CalendarIDs) != 1 || len(got.Tags) != 1 {
		t.Fatalf("SaveInsightInquiry = %#v, %v", got, err)
	}
	if err := svc.DeleteInsightInquiry(ctx, "daily_briefing"); err == nil || !strings.Contains(err.Error(), "built-in") {
		t.Fatalf("DeleteInsightInquiry(builtin) error = %v", err)
	}
	if _, err := svc.GetInsightInquiry(ctx, "school_today"); err != nil {
		t.Fatalf("GetInsightInquiry = %v", err)
	}
}

func TestRunInsightPersistsFailureAndUsesNamedInquiry(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: ptr(true), Endpoint: "http://127.0.0.1:1", Model: "test", APIKey: "test-key"})
	if err != nil {
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
