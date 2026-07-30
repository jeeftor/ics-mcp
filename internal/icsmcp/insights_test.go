package icsmcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
