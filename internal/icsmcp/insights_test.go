package icsmcp

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInsightInquiriesPersistScopeAndProtectBuiltins(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	enabled := false
	got, err := svc.SaveInsightInquiry(ctx, "school_today", SaveInsightInquiryInput{
		Question: "Do the kids have school today?", CalendarIDs: []string{"school"}, Tags: []string{"School"}, Schedule: "24h", Enabled: &enabled,
	})
	if err != nil || got.Enabled || got.Schedule != "24h" || len(got.CalendarIDs) != 1 || len(got.Tags) != 1 {
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
