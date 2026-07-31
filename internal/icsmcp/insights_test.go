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

func TestInsightInferenceUsesLongerDeadlineAndCompactGroundingPayload(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	svc.SetClock(func() time.Time { return time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC) })
	var deadlines []time.Duration
	var bodies [][]byte
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		deadline, ok := req.Context().Deadline()
		if !ok {
			t.Fatal("LLM request has no deadline")
		}
		deadlines = append(deadlines, time.Until(deadline))
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, body)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{\"answer\":\"OK\",\"evidence\":[]}"}}]}`))}, nil
	})}
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 7, 30, 15, 0, 0, 0, time.UTC)
	if err := svc.store.replaceEvents(ctx, cal.ID, []EventInstance{{ID: "event-1", UID: "uid-1", Name: "Private planning", Description: "do not send", MeetingURL: "https://secret.example", RecurrenceID: "recurrence-1", Start: start, End: start.Add(time.Hour), CalendarID: cal.ID, CalendarName: "Work"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: ptr(true), Endpoint: "http://llm.test/v1", Model: "test"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.TestLLMModel(ctx, LLMModelTestInput{Model: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PreviewInsight(ctx, RunInsightInput{Question: "What should I know?", DateScope: InsightDateScopeToday}); err != nil {
		t.Fatal(err)
	}
	if len(deadlines) != 2 || deadlines[0] > llmProbeTimeout || deadlines[1] > llmInferenceTimeout || deadlines[1] < time.Minute || deadlines[0] > 30*time.Second {
		t.Fatalf("LLM request deadlines = %#v, want probe about %s and inference about %s", deadlines, llmProbeTimeout, llmInferenceTimeout)
	}
	var testPayload, insightPayload map[string]any
	if err := json.Unmarshal(bodies[0], &testPayload); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(bodies[1], &insightPayload); err != nil {
		t.Fatal(err)
	}
	if got := testPayload["max_tokens"]; got != float64(4) {
		t.Fatalf("model-test max_tokens = %#v, want 4", got)
	}
	if got := insightPayload["max_tokens"]; got != float64(llmMaxOutputTokens) {
		t.Fatalf("inference max_tokens = %#v, want %d", got, llmMaxOutputTokens)
	}
	encoded := string(bodies[1])
	for _, absent := range []string{"do not send", "secret.example", "recurrence-1", "meeting_url", "description", "recurrence_id", "attendance_status"} {
		if strings.Contains(encoded, absent) {
			t.Fatalf("insight payload leaked %q: %s", absent, encoded)
		}
	}
	for _, present := range []string{"event-1", "Private planning", `\"start\"`, `\"end\"`, `\"calendar\"`, `\"all_day\"`, `\"cancelled\"`} {
		if !strings.Contains(encoded, present) {
			t.Fatalf("insight payload omitted %q: %s", present, encoded)
		}
	}
}

func TestInsightOllamaPayloadCapsGeneratedOutput(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	var body []byte
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"message":{"content":"{\"answer\":\"OK\",\"evidence\":[]}"}}`))}, nil
	})}
	if _, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: ptr(true), Backend: LLMBackendOllama, Endpoint: "http://ollama.test", Model: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PreviewInsight(ctx, RunInsightInput{Question: "What should I know?"}); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["max_tokens"]; exists {
		t.Fatalf("Ollama payload unexpectedly has max_tokens: %s", body)
	}
	options, ok := payload["options"].(map[string]any)
	if !ok || options["num_predict"] != float64(llmMaxOutputTokens) || payload["stream"] != false {
		t.Fatalf("Ollama payload = %s, want stream=false and num_predict=%d", body, llmMaxOutputTokens)
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

func TestLemonadeLifecycleUnavailableDoesNotPretendTheServerIsUnreachable(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/health":
			http.NotFound(w, r)
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	svc := newTestService(t)
	input := LLMModelTestInput{LLMConnectionInput: LLMConnectionInput{Backend: LLMBackendLemonade, Endpoint: provider.URL}, Model: "lemonade-model"}
	got, err := svc.LemonadeModelStatus(context.Background(), input)
	if err != nil || got.State != LemonadeModelStateLifecycleUnavailable {
		t.Fatalf("LemonadeModelStatus = %#v, %v", got, err)
	}
	if strings.Contains(strings.ToLower(got.Message), "could not be reached") {
		t.Fatalf("lifecycle-unavailable message implied a connectivity failure: %q", got.Message)
	}
	if err := svc.TestLLMModel(context.Background(), input); err != nil {
		t.Fatalf("TestLLMModel should still work when lifecycle API is absent: %v", err)
	}
	if err := svc.TestLLMEndpoint(context.Background(), input.LLMConnectionInput); err != nil {
		t.Fatalf("TestLLMEndpoint should report an HTTP response as reachable even without model discovery: %v", err)
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

// contextAwareBody simulates a real streaming HTTP response body. Unlike the
// in-memory bodies used by other tests, reads surface the request context
// error once it is canceled, exactly like a body read from a live socket.
type contextAwareBody struct {
	ctx     context.Context
	content []byte
	read    int
}

func (b *contextAwareBody) Read(p []byte) (int, error) {
	if err := b.ctx.Err(); err != nil {
		return 0, err
	}
	if b.read >= len(b.content) {
		return 0, io.EOF
	}
	n := copy(p, b.content[b.read:])
	b.read += n
	return n, nil
}

func (b *contextAwareBody) Close() error { return nil }

// TestDoLLMRequestKeepsContextAliveWhileBodyIsRead reproduces the dockarr
// failure where a real streaming LLM response body could not be decoded
// because doLLMRequest canceled the bounded request context before the caller
// read the body, producing "decode LLM response: context canceled". The in-
// memory bodies used elsewhere cannot reproduce this; only a context-aware
// streaming body can.
func TestDoLLMRequestKeepsContextAliveWhileBodyIsRead(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: &contextAwareBody{ctx: req.Context(), content: []byte(`{"choices":[{"message":{"content":"{\"answer\":\"OK\",\"evidence\":[]}"}}]}`)}}, nil
	})}
	if _, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: ptr(true), Endpoint: "http://llm.test/v1", Model: "test"}); err != nil {
		t.Fatal(err)
	}
	// Model discovery reads the body with io.ReadAll, so it is also broken when
	// the context is canceled before the body is read. The fake body is not a
	// models listing, but reading it to completion must not error.
	if _, err := svc.DiscoverLLMModels(ctx, LLMConnectionInput{Endpoint: "http://llm.test/v1"}); err != nil {
		t.Fatalf("DiscoverLLMModels failed to read streaming body: %v", err)
	}
	insight, err := svc.PreviewInsight(ctx, RunInsightInput{Question: "What should I know?"})
	if err != nil {
		t.Fatalf("PreviewInsight failed to decode streaming body: %v", err)
	}
	if insight.Answer != "OK" {
		t.Fatalf("PreviewInsight answer = %q, want OK", insight.Answer)
	}
}

// TestInsightFallsBackToReasoningContent reproduces the "LLM returned no
// answer" failure seen against reasoning models (DeepSeek-R1, Qwen-QwQ,
// o1-style, some Lemonade models) that populate message.reasoning_content
// while leaving message.content empty or null. The service must fall back to
// reasoning_content so the Insight still completes.
func TestInsightFallsBackToReasoningContent(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"","reasoning_content":"{\"answer\":\"From reasoning\",\"evidence\":[\"e1\"],\"caveat\":\"\"}"}}]}`))}, nil
	})}
	if _, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: ptr(true), Endpoint: "http://llm.test/v1", Model: "reasoning-model"}); err != nil {
		t.Fatal(err)
	}
	insight, err := svc.PreviewInsight(ctx, RunInsightInput{Question: "What should I know?"})
	if err != nil {
		t.Fatalf("PreviewInsight with reasoning_content failed: %v", err)
	}
	if insight.Answer != "From reasoning" {
		t.Fatalf("PreviewInsight reasoning fallback answer = %q, want From reasoning", insight.Answer)
	}
}

// TestInsightNoAnswerErrorReportsDiagnostics ensures the "no answer" failure
// surfaces enough structural detail to diagnose the provider response without
// leaking secrets or calendar data.
func TestInsightNoAnswerErrorReportsDiagnostics(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{},"finish_reason":"length"}]}`))}, nil
	})}
	if _, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: ptr(true), Endpoint: "http://llm.test/v1", Model: "test"}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.PreviewInsight(ctx, RunInsightInput{Question: "What should I know?"})
	if err == nil {
		t.Fatal("PreviewInsight error = nil, want no-answer diagnostic")
	}
	msg := err.Error()
	for _, want := range []string{"200", "choices"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("no-answer error missing %q: %q", want, msg)
		}
	}
}

// TestInsightExtractsJSONFromReasoningProse reproduces the real Qwen3.6
// reasoning-model response captured from a Lemonade deployment: content is
// empty, reasoning_content contains a chain-of-thought trace with the JSON
// answer embedded in a markdown code fence. The service must extract the JSON
// object from the prose rather than failing with "LLM answer must be JSON".
func TestInsightExtractsJSONFromReasoningProse(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	reasoning := "Here's a thinking process:\n\n1. Analyze...\n2. Evaluate...\n   ```json\n   {\n     \"answer\": \"No events today.\",\n     \"evidence\": [],\n     \"caveat\": \"Calendar data was empty.\"\n   }\n   ```\n\nAll constraints met."
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"length","index":0,"message":{"content":"","reasoning_content":` + jsonQuote(reasoning) + `}}]}`))}, nil
	})}
	if _, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: ptr(true), Backend: LLMBackendLemonade, Endpoint: "http://llm.test", Model: "Qwen3.6"}); err != nil {
		t.Fatal(err)
	}
	insight, err := svc.PreviewInsight(ctx, RunInsightInput{Question: "What should I know today?"})
	if err != nil {
		t.Fatalf("PreviewInsight with embedded JSON reasoning failed: %v", err)
	}
	if insight.Answer != "No events today." {
		t.Fatalf("PreviewInsight answer = %q, want %q", insight.Answer, "No events today.")
	}
	if insight.Caveat != "Calendar data was empty." {
		t.Fatalf("PreviewInsight caveat = %q, want %q", insight.Caveat, "Calendar data was empty.")
	}
}

// TestInsightReasoningModelGetsHigherTokenCap ensures reasoning models, which
// emit a chain-of-thought trace before the final answer, are not capped at the
// small llmMaxOutputTokens budget that only covers a direct JSON answer.
func TestInsightReasoningModelGetsHigherTokenCap(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	var capturedPayload map[string]any
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(body, &capturedPayload)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{\"answer\":\"OK\",\"evidence\":[]}"}}]}`))}, nil
	})}
	if _, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: ptr(true), Backend: LLMBackendLemonade, Endpoint: "http://llm.test", Model: "Qwen3.6"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PreviewInsight(ctx, RunInsightInput{Question: "What should I know?"}); err != nil {
		t.Fatal(err)
	}
	maxTokens, ok := capturedPayload["max_tokens"]
	if !ok {
		t.Fatalf("payload missing max_tokens: %v", capturedPayload)
	}
	if maxTokens.(float64) <= float64(llmMaxOutputTokens) {
		t.Fatalf("reasoning model max_tokens = %v, want > %d", maxTokens, llmMaxOutputTokens)
	}
}

// jsonQuote returns a JSON-quoted string literal for embedding in test fixtures.
func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// TestInsightPromptIncludesCurrentDateAndTimezone ensures the LLM prompt
// includes the current date and configured timezone so the model can reason
// about "today" in the user's local time, and does not use the word
// "untrusted" which the model echoes back into the caveat.
func TestInsightPromptIncludesCurrentDateAndTimezone(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	var capturedPayload map[string]any
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(body, &capturedPayload)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{\"answer\":\"OK\",\"evidence\":[]}"}}]}`))}, nil
	})}
	if _, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: ptr(true), Endpoint: "http://llm.test/v1", Model: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PreviewInsight(ctx, RunInsightInput{Question: "What should I know today?"}); err != nil {
		t.Fatal(err)
	}
	messages, ok := capturedPayload["messages"].([]any)
	if !ok || len(messages) < 2 {
		t.Fatalf("payload missing messages: %v", capturedPayload)
	}
	systemMsg, _ := messages[0].(map[string]any)
	userMsg, _ := messages[1].(map[string]any)
	systemContent, _ := systemMsg["content"].(string)
	userContent, _ := userMsg["content"].(string)
	combined := systemContent + "\n" + userContent
	if strings.Contains(strings.ToLower(combined), "untrusted") {
		t.Errorf("prompt still contains 'untrusted' (model echoes it back): %q", combined)
	}
	// The prompt must include the current date so the model knows what "today" is.
	if !strings.Contains(combined, "2026") {
		t.Errorf("prompt missing current date: %q", combined)
	}
	// The prompt must include the timezone so the model can reason about local time.
	if !strings.Contains(combined, "UTC") {
		t.Errorf("prompt missing timezone: %q", combined)
	}
}

func TestCronScheduleParsing(t *testing.T) {
	tests := []struct {
		expr string
		ok   bool
	}{
		{"0 6 * * *", true},       // daily at 6 AM
		{"*/15 * * * *", true},    // every 15 minutes
		{"0 9-17 * * 1-5", true},  // hourly 9-5 weekdays
		{"30 6 1 * *", true},      // 6:30 AM on the 1st of each month
		{"0 0 * * 0", true},       // midnight Sunday
		{"0 0 * * 7", true},       // 7 is also Sunday
		{"0,30 * * * *", true},    // every 30 minutes
		{"0 6 * *", false},        // only 4 fields
		{"0 6 * * * *", false},    // 6 fields
		{"60 6 * * *", false},     // minute out of range
		{"0 24 * * *", false},     // hour out of range
		{"0 6 0 * *", false},      // day-of-month out of range
		{"0 6 * 13 *", false},     // month out of range
		{"", false},               // empty
		{"abc", false},            // garbage
	}
	for _, tt := range tests {
		_, err := parseCronExpression(tt.expr)
		if tt.ok && err != nil {
			t.Errorf("parseCronExpression(%q) unexpected error: %v", tt.expr, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("parseCronExpression(%q) expected error, got nil", tt.expr)
		}
	}
}

func TestCronDueLogic(t *testing.T) {
	loc := time.UTC
	// "0 6 * * *" = daily at 6:00 AM
	schedule, err := parseCronExpression("0 6 * * *")
	if err != nil {
		t.Fatal(err)
	}
	// Never run, now is 6:00 AM → due
	now := time.Date(2026, 7, 30, 6, 0, 0, 0, loc)
	if !cronDue(schedule, time.Time{}, now) {
		t.Fatal("cron should be due at 6:00 AM when never run")
	}
	// Last run was today at 6:00 AM, now is 6:01 → not due (already ran today)
	lastRun := time.Date(2026, 7, 30, 6, 0, 0, 0, loc)
	now = time.Date(2026, 7, 30, 6, 1, 0, 0, loc)
	if cronDue(schedule, lastRun, now) {
		t.Fatal("cron should not be due again same day after running at 6:00")
	}
	// Last run was yesterday at 6:00, now is today at 6:00 → due
	lastRun = time.Date(2026, 7, 29, 6, 0, 0, 0, loc)
	now = time.Date(2026, 7, 30, 6, 0, 0, 0, loc)
	if !cronDue(schedule, lastRun, now) {
		t.Fatal("cron should be due next day at 6:00")
	}
	// Now is 5:59 AM, last run was yesterday 6:00 → not due yet
	now = time.Date(2026, 7, 30, 5, 59, 0, 0, loc)
	if cronDue(schedule, lastRun, now) {
		t.Fatal("cron should not be due before 6:00")
	}
	// "*/15 * * * *" = every 15 minutes
	schedule15, _ := parseCronExpression("*/15 * * * *")
	// Last run at :00, now is :14 → not due
	lastRun = time.Date(2026, 7, 30, 6, 0, 0, 0, loc)
	now = time.Date(2026, 7, 30, 6, 14, 0, 0, loc)
	if cronDue(schedule15, lastRun, now) {
		t.Fatal("*/15 should not be due at :14 after running at :00")
	}
	// Last run at :00, now is :15 → due
	now = time.Date(2026, 7, 30, 6, 15, 0, 0, loc)
	if !cronDue(schedule15, lastRun, now) {
		t.Fatal("*/15 should be due at :15 after running at :00")
	}
}

func TestSaveInsightInquiryCronMode(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	// Valid cron expression
	saved, err := svc.SaveInsightInquiry(ctx, "cron_test", SaveInsightInquiryInput{
		Question:       "Test cron",
		Trigger:        InsightTriggerScheduled,
		ScheduleMode:   InsightScheduleModeCron,
		CronExpression: "0 6 * * *",
	})
	if err != nil {
		t.Fatalf("SaveInsightInquiry with cron failed: %v", err)
	}
	if saved.CronExpression != "0 6 * * *" {
		t.Fatalf("saved cron expression = %q, want %q", saved.CronExpression, "0 6 * * *")
	}
	// Invalid cron expression
	_, err = svc.SaveInsightInquiry(ctx, "cron_bad", SaveInsightInquiryInput{
		Question:       "Bad cron",
		Trigger:        InsightTriggerScheduled,
		ScheduleMode:   InsightScheduleModeCron,
		CronExpression: "not a cron",
	})
	if err == nil || !strings.Contains(err.Error(), "cron") {
		t.Fatalf("SaveInsightInquiry with bad cron expected error, got: %v", err)
	}
}
