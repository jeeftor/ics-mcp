package icsmcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPromptOutputAndHistoryAreCachedAndBounded(t *testing.T) {
	ctx := context.Background()
	var calls int
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"answer\":\"Tomorrow is a holiday.\",\"evidence\":[\"holiday-event\"],\"caveat\":\"Calendar data only.\"}"}}]}`))
	}))
	defer provider.Close()

	svc := newTestService(t)
	if _, err := svc.UpdateLLMProfile(ctx, UpdateLLMProfileInput{Enabled: ptr(true), Endpoint: provider.URL + "/v1", Model: "test-model"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveInsightInquiry(ctx, "tomorrow_holidays", SaveInsightInquiryInput{Question: "Are there holidays tomorrow?"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunInsight(ctx, RunInsightInput{Name: "tomorrow_holidays"}); err != nil {
		t.Fatal(err)
	}
	output, err := svc.GetPromptOutput(ctx, "tomorrow_holidays")
	if err != nil || output.ID != "tomorrow_holidays" || output.Text == "" || output.Result == "" || output.Model != "test-model" || output.RunAt.IsZero() || len(output.Evidence) != 1 {
		t.Fatalf("GetPromptOutput = %#v, %v", output, err)
	}
	if calls != 1 {
		t.Fatalf("cached output invoked provider %d times, want 1", calls)
	}
	for i := 0; i < 11; i++ {
		now := time.Date(2026, time.July, 30, 12, i, 0, 0, time.UTC)
		if err := svc.recordPromptRun(ctx, PromptRun{PromptID: "tomorrow_holidays", Text: "Are there holidays tomorrow?", Result: "run", SourceAt: now, RunAt: now, Model: "test-model"}); err != nil {
			t.Fatal(err)
		}
	}
	history, err := svc.ListPromptHistory(ctx, "tomorrow_holidays", 100)
	if err != nil || len(history) != 10 || history[0].PromptID != "tomorrow_holidays" || history[0].Model != "test-model" {
		t.Fatalf("ListPromptHistory = %#v, %v", history, err)
	}
}

func TestVersionedPromptRoutesAreReadOnlyAndReturnCachedOutput(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	if _, err := svc.SaveInsightInquiry(ctx, "school_tomorrow", SaveInsightInquiryInput{Question: "Do the kids have school tomorrow?"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 30, 12, 0, 0, 0, time.UTC)
	if err := svc.recordPromptRun(ctx, PromptRun{PromptID: "school_tomorrow", Text: "Do the kids have school tomorrow?", Result: "No school.", SourceAt: now, RunAt: now, Model: "test-model"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.store.db.ExecContext(ctx, `INSERT INTO insights (name, question, answer, source_hash, source_at, generated_at, model) VALUES (?, ?, ?, '', ?, ?, ?)`, "school_tomorrow", "Do the kids have school tomorrow?", "No school.", now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), "test-model"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	var output PromptOutput
	doJSON(t, http.MethodGet, server.URL+"/api/v1/prompts/school_tomorrow/output", nil, &output)
	if output.Result != "No school." || output.Model != "test-model" || output.ID != "school_tomorrow" {
		t.Fatalf("prompt output = %#v", output)
	}
	var history []PromptRun
	doJSON(t, http.MethodGet, server.URL+"/api/v1/prompts/school_tomorrow/history?limit=1", nil, &history)
	if len(history) != 1 || history[0].Result != "No school." {
		t.Fatalf("prompt history = %#v", history)
	}
	var list []PromptOutput
	doJSON(t, http.MethodGet, server.URL+"/api/v1/prompts", nil, &list)
	if len(list) < 1 {
		t.Fatalf("prompt list = %#v", list)
	}

	resp, err := http.Post(server.URL+"/api/v1/prompts/school_tomorrow/output", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST prompt output status = %d", resp.StatusCode)
	}
	encoded, _ := json.Marshal(output)
	if string(encoded) == "" {
		t.Fatal("prompt output did not marshal")
	}
}
