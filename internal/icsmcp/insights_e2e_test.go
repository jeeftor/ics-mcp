package icsmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestInsightLifecycleOverHTTPAndMCP exercises the complete optional insight
// lifecycle without a real provider or credential. It is deliberately
// transport-level so the console's REST calls and advertised MCP read tools
// remain aligned with the service contract.
func TestInsightLifecycleOverHTTPAndMCP(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 30, 15, 0, 0, 0, time.UTC)
	svc := newTestService(t)
	svc.SetClock(func() time.Time { return now })

	mode := "success"
	var seenTest, seenRun bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" && r.URL.Path != "/chat/completions" {
			t.Errorf("provider path = %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer local-test-key" {
			t.Errorf("provider authorization = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "private description") {
			t.Error("untrusted event description was included in LLM payload")
		}
		if strings.Contains(string(body), "Reply with OK.") {
			seenTest = true
		} else {
			seenRun = true
		}
		if mode == "failure" {
			http.Error(w, "provider unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"answer\":\"No school today\",\"evidence\":[\"school-event\"],\"caveat\":\"Based on the configured school calendar.\"}"}}]}`))
	}))
	defer provider.Close()

	calendar, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "school", Name: "School", URL: "https://example.test/school.ics", Tags: []string{"School"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ReplaceEvents(ctx, calendar.ID, []EventInstance{{
		ID: "school-event", UID: "school-event", Name: "No school", Description: "private description",
		Start: now.Add(24 * time.Hour), End: now.Add(25 * time.Hour),
	}}); err != nil {
		t.Fatal(err)
	}

	api := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer api.Close()

	var profile LLMProfile
	doJSON(t, http.MethodPut, api.URL+"/api/llm-profile", UpdateLLMProfileInput{
		Enabled: ptr(true), Endpoint: provider.URL + "/v1", Model: "fake-model", APIKey: "local-test-key",
	}, &profile)
	if !profile.Enabled || profile.Endpoint != provider.URL+"/v1" || profile.Model != "fake-model" || !profile.APIKeyConfigured || profile.Source != "database" {
		t.Fatalf("saved profile = %#v", profile)
	}
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(profileJSON), "local-test-key") {
		t.Fatalf("profile exposed API key: %s", profileJSON)
	}

	var profileTest map[string]bool
	doJSON(t, http.MethodPost, api.URL+"/api/llm-profile/test", map[string]any{}, &profileTest)
	if !profileTest["ok"] || !seenTest {
		t.Fatalf("profile test = %#v, provider seen=%v", profileTest, seenTest)
	}
	// An exact endpoint is also accepted; it must not become
	// /chat/completions/chat/completions at request time.
	doJSON(t, http.MethodPut, api.URL+"/api/llm-profile", UpdateLLMProfileInput{Endpoint: provider.URL + "/v1/chat/completions"}, &profile)
	if profile.Endpoint != provider.URL+"/v1/chat/completions" {
		t.Fatalf("exact endpoint profile = %#v", profile)
	}

	var inquiry InsightInquiry
	doJSON(t, http.MethodPost, api.URL+"/api/insight-inquiries", map[string]any{
		"name": "school_today", "question": "Do the kids have school today?", "calendar_ids": []string{calendar.ID}, "tags": []string{"School"}, "trigger": "scheduled", "schedule": "06:00",
	}, &inquiry)
	if inquiry.Name != "school_today" || inquiry.Question == "" || inquiry.Trigger != InsightTriggerScheduled || inquiry.Schedule != "06:00" || !inquiry.Enabled {
		t.Fatalf("saved inquiry = %#v", inquiry)
	}

	var ran Insight
	doJSON(t, http.MethodPost, api.URL+"/api/insights", map[string]any{"name": "school_today"}, &ran)
	if !seenRun || ran.Answer != "No school today" || len(ran.Evidence) != 1 || ran.Evidence[0] != "school-event" || ran.Caveat == "" || ran.SourceHash == "" || ran.GeneratedAt.IsZero() || ran.SourceAt.IsZero() || ran.Stale {
		t.Fatalf("run insight = %#v, provider seen=%v", ran, seenRun)
	}

	var cached Insight
	doJSON(t, http.MethodGet, api.URL+"/api/insights/school_today", nil, &cached)
	if cached.Answer != ran.Answer || cached.SourceHash != ran.SourceHash || cached.Error != "" {
		t.Fatalf("cached REST insight = %#v", cached)
	}

	// A newer calendar refresh makes the existing cached answer stale; the read
	// must report that state without making another provider call.
	if _, err := svc.store.db.ExecContext(ctx, `UPDATE refresh_state SET last_success = ? WHERE calendar_id = ?`, now.Add(time.Minute).Format(time.RFC3339Nano), calendar.ID); err != nil {
		t.Fatal(err)
	}
	var stale Insight
	doJSON(t, http.MethodGet, api.URL+"/api/insights/school_today", nil, &stale)
	if !stale.Stale || !seenRun {
		t.Fatalf("stale cached insight = %#v", stale)
	}

	session, err := mcp.NewClient(&mcp.Implementation{Name: "insight-e2e", Version: "v0.0.1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: api.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect MCP: %v", err)
	}
	defer session.Close()
	getResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_insight", Arguments: map[string]any{"name": "school_today"}})
	if err != nil {
		t.Fatalf("get_insight MCP: %v", err)
	}
	var mcpCached Insight
	decodeStructured(t, getResult.StructuredContent, &mcpCached)
	if mcpCached.Answer != ran.Answer || !mcpCached.Stale {
		t.Fatalf("get_insight MCP = %#v", mcpCached)
	}
	listResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_insights", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list_insights MCP: %v", err)
	}
	var mcpList []Insight
	decodeStructured(t, listResult.StructuredContent, &mcpList)
	if len(mcpList) != 1 || mcpList[0].Name != "school_today" || !mcpList[0].Stale {
		t.Fatalf("list_insights MCP = %#v", mcpList)
	}

	// Explicit execution failures are retained as cached errors and do not leak
	// the API key through either the HTTP error or the cached record.
	mode = "failure"
	requestBody := bytes.NewBufferString(`{"name":"school_today"}`)
	resp, err := http.Post(api.URL+"/api/insights", "application/json", requestBody)
	if err != nil {
		t.Fatal(err)
	}
	errorBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode < 400 || strings.Contains(string(errorBody), "local-test-key") {
		t.Fatalf("failed run status=%d body=%s", resp.StatusCode, errorBody)
	}
	var failed Insight
	doJSON(t, http.MethodGet, api.URL+"/api/insights/school_today", nil, &failed)
	if failed.Error == "" || failed.Answer != ran.Answer || strings.Contains(failed.Error, "local-test-key") {
		t.Fatalf("cached failure = %#v", failed)
	}
}
