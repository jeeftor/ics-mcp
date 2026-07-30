package icsmcp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// LLMProfile is the safe, redacted configuration returned to clients.
type LLMProfile struct {
	Enabled          bool   `json:"enabled"`
	Backend          string `json:"backend"`
	Endpoint         string `json:"endpoint,omitempty"`
	Model            string `json:"model,omitempty"`
	APIKeyConfigured bool   `json:"api_key_configured"`
	Source           string `json:"source"`
}

// UpdateLLMProfileInput changes the locally persisted optional LLM profile.
// Environment values take precedence and cannot be changed through this input.
type UpdateLLMProfileInput struct {
	Enabled  *bool  `json:"enabled,omitempty"`
	Backend  string `json:"backend,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Model    string `json:"model,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
}

// LLMConnectionInput is a temporary OpenAI-compatible connection. It is used
// only for staged admin checks and is never persisted or returned to clients.
type LLMConnectionInput struct {
	Backend  string `json:"backend,omitempty"`
	Endpoint string `json:"endpoint"`
	APIKey   string `json:"api_key"`
}

// LLMModelTestInput adds the selected model to a temporary connection check.
type LLMModelTestInput struct {
	LLMConnectionInput
	Model string `json:"model"`
}

// Insight is a cached, grounded answer. It never includes an API key or prompt.
type Insight struct {
	Name        string    `json:"name"`
	Question    string    `json:"question"`
	Answer      string    `json:"answer"`
	Evidence    []string  `json:"evidence"`
	Caveat      string    `json:"caveat,omitempty"`
	SourceHash  string    `json:"source_hash"`
	SourceAt    time.Time `json:"source_at"`
	GeneratedAt time.Time `json:"generated_at"`
	Error       string    `json:"error,omitempty"`
	Stale       bool      `json:"stale"`
}

// RunInsightInput explicitly requests an LLM call. Normal reads never do this.
type RunInsightInput struct {
	Name        string   `json:"name"`
	Question    string   `json:"question"`
	CalendarIDs []string `json:"calendar_ids,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// InsightTrigger controls when an enabled inquiry may invoke the model.
// Manual is always explicit, OnChange runs only after its scoped event data
// changes, and Scheduled runs once per local calendar day at Schedule.
type InsightTrigger string

const (
	InsightTriggerManual    InsightTrigger = "manual"
	InsightTriggerOnChange  InsightTrigger = "on_change"
	InsightTriggerScheduled InsightTrigger = "scheduled"
)

// InsightInquiry is a named, persisted request definition. Normal reads never
// invoke a model regardless of its trigger.
type InsightInquiry struct {
	Name        string         `json:"name"`
	Question    string         `json:"question"`
	CalendarIDs []string       `json:"calendar_ids,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Trigger     InsightTrigger `json:"trigger"`
	// Schedule is a daily local wall-clock time in HH:MM form for scheduled
	// inquiries. It uses the service's configured timezone.
	Schedule  string    `json:"schedule,omitempty"`
	Enabled   bool      `json:"enabled"`
	Builtin   bool      `json:"builtin"`
	LastRunAt time.Time `json:"last_run_at,omitempty"`
}

// SaveInsightInquiryInput creates or updates a named inquiry.
type SaveInsightInquiryInput struct {
	Question    string         `json:"question"`
	CalendarIDs []string       `json:"calendar_ids,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Trigger     InsightTrigger `json:"trigger,omitempty"`
	Schedule    string         `json:"schedule,omitempty"`
	Enabled     *bool          `json:"enabled,omitempty"`
}

type llmProfileSecret struct {
	LLMProfile
	apiKey string
}

const (
	LLMBackendOpenAI   = "openai"
	LLMBackendOllama   = "ollama"
	LLMBackendLemonade = "lemonade"
)

func (s *Service) llmProfile(ctx context.Context) (llmProfileSecret, error) {
	var enabled int
	var endpoint, model, key, backend string
	if err := s.store.db.QueryRowContext(ctx, `SELECT enabled, backend, endpoint, model, api_key FROM llm_profile WHERE id = 1`).Scan(&enabled, &backend, &endpoint, &model, &key); err != nil {
		return llmProfileSecret{}, fmt.Errorf("load llm profile: %w", err)
	}
	p := llmProfileSecret{LLMProfile: LLMProfile{Enabled: enabled != 0, Backend: normalizeLLMBackend(backend), Endpoint: endpoint, Model: model, APIKeyConfigured: key != "", Source: "database"}, apiKey: key}
	if value, ok := os.LookupEnv("ICSMCP_LLM_ENABLED"); ok {
		p.Enabled = strings.EqualFold(value, "true") || value == "1"
		p.Source = "environment"
	}
	if value, ok := os.LookupEnv("ICSMCP_LLM_ENDPOINT"); ok {
		p.Endpoint, p.Source = strings.TrimSpace(value), "environment"
	}
	if value, ok := os.LookupEnv("ICSMCP_LLM_BACKEND"); ok {
		p.Backend, p.Source = normalizeLLMBackend(value), "environment"
	}
	if value, ok := os.LookupEnv("ICSMCP_LLM_MODEL"); ok {
		p.Model, p.Source = strings.TrimSpace(value), "environment"
	}
	if value, ok := os.LookupEnv("ICSMCP_LLM_API_KEY"); ok {
		p.apiKey, p.APIKeyConfigured, p.Source = value, value != "", "environment"
	}
	return p, nil
}

// LLMProfile returns a redacted effective optional LLM profile.
func (s *Service) LLMProfile(ctx context.Context) (LLMProfile, error) {
	p, err := s.llmProfile(ctx)
	return p.LLMProfile, err
}

// UpdateLLMProfile persists a profile without exposing its secret.
func (s *Service) UpdateLLMProfile(ctx context.Context, in UpdateLLMProfileInput) (LLMProfile, error) {
	current, err := s.llmProfile(ctx)
	if err != nil {
		return LLMProfile{}, err
	}
	if current.Source == "environment" {
		return LLMProfile{}, fmt.Errorf("LLM profile is overridden by environment")
	}
	if in.Enabled != nil {
		current.Enabled = *in.Enabled
	}
	if in.Backend != "" {
		if !validLLMBackend(in.Backend) {
			return LLMProfile{}, fmt.Errorf("LLM backend must be openai, ollama, or lemonade")
		}
		current.Backend = normalizeLLMBackend(in.Backend)
	}
	if strings.TrimSpace(in.Endpoint) != "" {
		current.Endpoint = strings.TrimSpace(in.Endpoint)
	}
	if strings.TrimSpace(in.Model) != "" {
		current.Model = strings.TrimSpace(in.Model)
	}
	if in.APIKey != "" {
		current.apiKey = in.APIKey
	}
	if _, err := s.store.db.ExecContext(ctx, `UPDATE llm_profile SET enabled = ?, backend = ?, endpoint = ?, model = ?, api_key = ? WHERE id = 1`, boolInt(current.Enabled), current.Backend, current.Endpoint, current.Model, current.apiKey); err != nil {
		return LLMProfile{}, fmt.Errorf("save llm profile: %w", err)
	}
	current.APIKeyConfigured = current.apiKey != ""
	return current.LLMProfile, nil
}

// TestLLMProfile verifies the effective profile without returning or logging its key.
func (s *Service) TestLLMProfile(ctx context.Context) error {
	p, err := s.llmProfile(ctx)
	if err != nil {
		return err
	}
	if !p.Enabled || p.Endpoint == "" || p.Model == "" {
		return fmt.Errorf("LLM endpoint, model, and enabled state are required")
	}
	return s.testLLMModel(ctx, p, p.Model)
}

// TestLLMEndpoint verifies an unsaved endpoint and optional bearer key. When
// the active profile is environment-managed, that effective profile remains
// authoritative and user-provided staging values are ignored.
func (s *Service) TestLLMEndpoint(ctx context.Context, in LLMConnectionInput) error {
	_, err := s.DiscoverLLMModels(ctx, in)
	return err
}

// DiscoverLLMModels reads IDs from an OpenAI-compatible GET /models response.
// It never persists or returns the bearer key.
func (s *Service) DiscoverLLMModels(ctx context.Context, in LLMConnectionInput) ([]string, error) {
	p, err := s.stagedLLMProfile(ctx, in)
	if err != nil {
		return nil, err
	}
	if p.Endpoint == "" {
		return nil, fmt.Errorf("LLM endpoint is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, llmModelsURL(p.Backend, p.Endpoint), nil)
	if err != nil {
		return nil, fmt.Errorf("create LLM model discovery request: %w", err)
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover LLM models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("LLM model discovery returned %s", resp.Status)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if p.Backend == LLMBackendOllama {
		var ollama struct { Models []struct { Name string `json:"name"` } `json:"models"` }
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&ollama); err != nil { return nil, fmt.Errorf("decode Ollama model discovery response: %w", err) }
		for _, item := range ollama.Models { payload.Data = append(payload.Data, struct { ID string `json:"id"` }{ID: item.Name}) }
	} else if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode LLM model discovery response: %w", err)
	}
	models := make([]string, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" || len(id) > 256 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, id)
		if len(models) == 500 {
			break
		}
	}
	return models, nil
}

// TestLLMModel sends a minimal completion using unsaved staged values.
func (s *Service) TestLLMModel(ctx context.Context, in LLMModelTestInput) error {
	p, err := s.stagedLLMProfile(ctx, in.LLMConnectionInput)
	if err != nil {
		return err
	}
	return s.testLLMModel(ctx, p, strings.TrimSpace(in.Model))
}

func (s *Service) stagedLLMProfile(ctx context.Context, in LLMConnectionInput) (llmProfileSecret, error) {
	p, err := s.llmProfile(ctx)
	if err != nil || p.Source == "environment" {
		return p, err
	}
	if endpoint := strings.TrimSpace(in.Endpoint); endpoint != "" {
		p.Endpoint = endpoint
	}
	if in.Backend != "" && validLLMBackend(in.Backend) { p.Backend = normalizeLLMBackend(in.Backend) }
	if in.APIKey != "" {
		p.apiKey = in.APIKey
		p.APIKeyConfigured = true
	}
	return p, nil
}

func (s *Service) testLLMModel(ctx context.Context, p llmProfileSecret, model string) error {
	if p.Endpoint == "" || model == "" {
		return fmt.Errorf("LLM endpoint and model are required")
	}
	payload := map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": "Reply with OK."}}, "max_tokens": 4}
	if p.Backend == LLMBackendOllama { payload["stream"] = false; delete(payload, "max_tokens") }
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, llmChatURL(p.Backend, p.Endpoint), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create LLM model test request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("test LLM model: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("LLM model test returned %s", resp.Status)
	}
	return nil
}

// ListInsightInquiries returns saved inquiry definitions, never invoking an LLM.
func (s *Service) ListInsightInquiries(ctx context.Context) ([]InsightInquiry, error) {
	rows, err := s.store.db.QueryContext(ctx, `SELECT name, question, calendar_ids_json, tags_json, trigger, schedule, enabled, builtin, last_run_at FROM insight_inquiries ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list insight inquiries: %w", err)
	}
	defer rows.Close()
	var out []InsightInquiry
	for rows.Next() {
		var item InsightInquiry
		var calendars, tags, lastRun string
		var enabled, builtin int
		if err := rows.Scan(&item.Name, &item.Question, &calendars, &tags, &item.Trigger, &item.Schedule, &enabled, &builtin, &lastRun); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(calendars), &item.CalendarIDs)
		_ = json.Unmarshal([]byte(tags), &item.Tags)
		item.Enabled, item.Builtin = enabled != 0, builtin != 0
		if lastRun != "" {
			item.LastRunAt, err = time.Parse(time.RFC3339Nano, lastRun)
			if err != nil {
				return nil, fmt.Errorf("parse inquiry last run: %w", err)
			}
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// GetInsightInquiry finds a saved named inquiry.
func (s *Service) GetInsightInquiry(ctx context.Context, name string) (InsightInquiry, error) {
	items, err := s.ListInsightInquiries(ctx)
	if err != nil {
		return InsightInquiry{}, err
	}
	for _, item := range items {
		if item.Name == name {
			return item, nil
		}
	}
	return InsightInquiry{}, sql.ErrNoRows
}

// SaveInsightInquiry creates or updates a safe bounded inquiry definition.
func (s *Service) SaveInsightInquiry(ctx context.Context, name string, in SaveInsightInquiryInput) (InsightInquiry, error) {
	name = strings.TrimSpace(name)
	in.Question = strings.TrimSpace(in.Question)
	if name == "" || in.Question == "" {
		return InsightInquiry{}, fmt.Errorf("name and question are required")
	}
	if strings.ContainsAny(name, "/\\") {
		return InsightInquiry{}, fmt.Errorf("inquiry name must not contain a path separator")
	}
	calendars, _ := json.Marshal(in.CalendarIDs)
	tags, _ := json.Marshal(in.Tags)
	existing, err := s.GetInsightInquiry(ctx, name)
	if err != nil && err != sql.ErrNoRows {
		return InsightInquiry{}, err
	}
	enabled := true
	builtin := false
	if err == nil {
		enabled, builtin = existing.Enabled, existing.Builtin
		if in.Trigger == "" {
			in.Trigger = existing.Trigger
		}
	}
	if in.Trigger == "" {
		in.Trigger = InsightTriggerManual
	}
	if !validInsightTrigger(in.Trigger) {
		return InsightInquiry{}, fmt.Errorf("trigger must be manual, on_change, or scheduled")
	}
	if in.Trigger == InsightTriggerScheduled {
		if _, _, err := parseDailyInsightTime(in.Schedule); err != nil {
			return InsightInquiry{}, err
		}
	} else {
		in.Schedule = ""
	}
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	_, err = s.store.db.ExecContext(ctx, `INSERT INTO insight_inquiries (name, question, calendar_ids_json, tags_json, trigger, schedule, enabled, builtin) VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(name) DO UPDATE SET question=excluded.question, calendar_ids_json=excluded.calendar_ids_json, tags_json=excluded.tags_json, trigger=excluded.trigger, schedule=excluded.schedule, enabled=excluded.enabled`, name, in.Question, string(calendars), string(tags), in.Trigger, in.Schedule, boolInt(enabled), boolInt(builtin))
	if err != nil {
		return InsightInquiry{}, fmt.Errorf("save insight inquiry: %w", err)
	}
	return s.GetInsightInquiry(ctx, name)
}

// DeleteInsightInquiry removes a custom definition and its cached output. Built-ins remain available.
func (s *Service) DeleteInsightInquiry(ctx context.Context, name string) error {
	inquiry, err := s.GetInsightInquiry(ctx, name)
	if err != nil {
		return err
	}
	if inquiry.Builtin {
		return fmt.Errorf("built-in inquiry cannot be deleted")
	}
	if _, err := s.store.db.ExecContext(ctx, `DELETE FROM insight_inquiries WHERE name = ?`, name); err != nil {
		return fmt.Errorf("delete insight inquiry: %w", err)
	}
	_, err = s.store.db.ExecContext(ctx, `DELETE FROM insights WHERE name = ?`, name)
	return err
}

// ListInsights returns cached insight results only.
func (s *Service) ListInsights(ctx context.Context) ([]Insight, error) {
	rows, err := s.store.db.QueryContext(ctx, `SELECT name, question, answer, evidence_json, caveat, source_hash, source_at, generated_at, error FROM insights ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list insights: %w", err)
	}
	defer rows.Close()
	out := []Insight{}
	for rows.Next() {
		var i Insight
		var evidence, sourceAt, generatedAt string
		if err := rows.Scan(&i.Name, &i.Question, &i.Answer, &evidence, &i.Caveat, &i.SourceHash, &sourceAt, &generatedAt, &i.Error); err != nil {
			return nil, err
		}
		var err error
		i.SourceAt, err = time.Parse(time.RFC3339Nano, sourceAt)
		if err != nil {
			return nil, fmt.Errorf("parse insight source timestamp: %w", err)
		}
		i.GeneratedAt, err = time.Parse(time.RFC3339Nano, generatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse insight generation timestamp: %w", err)
		}
		_ = json.Unmarshal([]byte(evidence), &i.Evidence)
		i.Stale, err = s.insightStale(ctx, i.GeneratedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Service) insightStale(ctx context.Context, generatedAt time.Time) (bool, error) {
	var latest sql.NullString
	if err := s.store.db.QueryRowContext(ctx, `SELECT MAX(last_success) FROM refresh_state`).Scan(&latest); err != nil {
		return false, fmt.Errorf("read insight freshness: %w", err)
	}
	if !latest.Valid || latest.String == "" {
		return false, nil
	}
	refreshedAt, err := time.Parse(time.RFC3339Nano, latest.String)
	if err != nil {
		return false, fmt.Errorf("parse insight refresh timestamp: %w", err)
	}
	return refreshedAt.After(generatedAt), nil
}

// GetInsight returns one cached result and never invokes an LLM.
func (s *Service) GetInsight(ctx context.Context, name string) (Insight, error) {
	items, err := s.ListInsights(ctx)
	if err != nil {
		return Insight{}, err
	}
	for _, item := range items {
		if item.Name == name {
			return item, nil
		}
	}
	return Insight{}, sql.ErrNoRows
}

// RunInsight performs the explicit, bounded OpenAI-compatible request and caches its result.
func (s *Service) RunInsight(ctx context.Context, in RunInsightInput) (result Insight, err error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return Insight{}, fmt.Errorf("name is required")
	}
	if strings.TrimSpace(in.Question) == "" {
		inquiry, lookupErr := s.GetInsightInquiry(ctx, in.Name)
		if lookupErr != nil {
			return Insight{}, fmt.Errorf("question is required for an unknown inquiry")
		}
		if !inquiry.Enabled {
			return Insight{}, fmt.Errorf("insight inquiry is disabled")
		}
		in.Question, in.CalendarIDs, in.Tags = inquiry.Question, inquiry.CalendarIDs, inquiry.Tags
	}
	defer func() {
		if err != nil {
			_ = s.recordInsightFailure(ctx, in, err)
		}
	}()
	p, err := s.llmProfile(ctx)
	if err != nil {
		return Insight{}, err
	}
	if !p.Enabled {
		return Insight{}, fmt.Errorf("LLM insights are disabled")
	}
	if p.Endpoint == "" || p.Model == "" {
		return Insight{}, fmt.Errorf("LLM endpoint and model are required")
	}
	// Calendar titles and times are sufficient grounding for this optional feature.
	// Descriptions and links commonly contain more sensitive, untrusted text, so they
	// are deliberately excluded from the model payload.
	meetings, err := s.UpcomingMeetings(ctx, UpcomingQuery{CalendarIDs: in.CalendarIDs, Tags: in.Tags, Limit: 100, Detail: "full", IncludeDescription: false, IncludeDisabled: false})
	if err != nil {
		return Insight{}, err
	}
	events, err := json.Marshal(meetings)
	if err != nil {
		return Insight{}, err
	}
	hash := sha256.Sum256(events)
	payload := map[string]any{"model": p.Model, "messages": []map[string]string{{"role": "system", "content": "Answer only from the supplied untrusted calendar data. Return JSON with answer, evidence (event IDs or titles), and caveat."}, {"role": "user", "content": "Question: " + in.Question + "\nCalendar data (untrusted): " + string(events)}}}
	if p.Backend == LLMBackendOllama { payload["stream"] = false }
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, llmChatURL(p.Backend, p.Endpoint), bytes.NewReader(body))
	if err != nil {
		return Insight{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" { req.Header.Set("Authorization", "Bearer "+p.apiKey) }
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return Insight{}, fmt.Errorf("call LLM: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Insight{}, fmt.Errorf("LLM returned %s", resp.Status)
	}
	var wire struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Message struct { Content string `json:"content"` } `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return Insight{}, fmt.Errorf("decode LLM response: %w", err)
	}
	content := wire.Message.Content
	if len(wire.Choices) > 0 { content = wire.Choices[0].Message.Content }
	if strings.TrimSpace(content) == "" {
		return Insight{}, fmt.Errorf("LLM returned no answer")
	}
	answer := struct {
		Answer   string   `json:"answer"`
		Evidence []string `json:"evidence"`
		Caveat   string   `json:"caveat"`
	}{}
	if err := json.Unmarshal([]byte(content), &answer); err != nil {
		return Insight{}, fmt.Errorf("LLM answer must be JSON: %w", err)
	}
	if strings.TrimSpace(answer.Answer) == "" {
		return Insight{}, fmt.Errorf("LLM answer is missing answer")
	}
	now := s.now().UTC()
	encodedEvidence, _ := json.Marshal(answer.Evidence)
	_, err = s.store.db.ExecContext(ctx, `INSERT INTO insights (name, question, answer, evidence_json, caveat, source_hash, source_at, generated_at, error) VALUES (?, ?, ?, ?, ?, ?, ?, ?, '') ON CONFLICT(name) DO UPDATE SET question=excluded.question, answer=excluded.answer, evidence_json=excluded.evidence_json, caveat=excluded.caveat, source_hash=excluded.source_hash, source_at=excluded.source_at, generated_at=excluded.generated_at, error=''`, in.Name, in.Question, answer.Answer, string(encodedEvidence), answer.Caveat, hex.EncodeToString(hash[:]), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return Insight{}, fmt.Errorf("cache insight: %w", err)
	}
	if _, err = s.store.db.ExecContext(ctx, `UPDATE insight_inquiries SET last_run_at = ? WHERE name = ?`, now.Format(time.RFC3339Nano), in.Name); err != nil {
		return Insight{}, fmt.Errorf("record inquiry run: %w", err)
	}
	return s.GetInsight(ctx, in.Name)
}

// llmChatCompletionsURL accepts either an OpenAI-compatible API base such as
// https://provider.example/v1 or the provider's exact chat completions URL.
// CamSpeak deployments commonly publish the latter directly.
func llmChatCompletionsURL(endpoint string) string {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if strings.HasSuffix(strings.ToLower(endpoint), "/chat/completions") {
		return endpoint
	}
	return endpoint + "/chat/completions"
}

// llmModelsURL maps either an API base or a full chat-completions endpoint to
// the corresponding OpenAI-compatible models collection.
func llmModelsURL(backend, endpoint string) string {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if normalizeLLMBackend(backend) == LLMBackendOllama {
		if strings.HasSuffix(strings.ToLower(endpoint), "/api/chat") { endpoint = endpoint[:len(endpoint)-len("/api/chat")] }
		return endpoint + "/api/tags"
	}
	if strings.HasSuffix(strings.ToLower(endpoint), "/chat/completions") {
		return endpoint[:len(endpoint)-len("/chat/completions")] + "/models"
	}
	if strings.HasSuffix(strings.ToLower(endpoint), "/models") {
		return endpoint
	}
	return endpoint + "/models"
}

func llmChatURL(backend, endpoint string) string {
	if normalizeLLMBackend(backend) != LLMBackendOllama { return llmChatCompletionsURL(endpoint) }
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if strings.HasSuffix(strings.ToLower(endpoint), "/api/chat") { return endpoint }
	return endpoint + "/api/chat"
}

func normalizeLLMBackend(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), LLMBackendOllama) { return LLMBackendOllama }
	if strings.EqualFold(strings.TrimSpace(value), LLMBackendLemonade) { return LLMBackendLemonade }
	return LLMBackendOpenAI
}

func validLLMBackend(value string) bool { return normalizeLLMBackend(value) == strings.ToLower(strings.TrimSpace(value)) }

func (s *Service) recordInsightFailure(ctx context.Context, in RunInsightInput, runErr error) error {
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err := s.store.db.ExecContext(ctx, `INSERT INTO insights (name, question, answer, source_hash, source_at, generated_at, error) VALUES (?, ?, '', '', ?, ?, ?) ON CONFLICT(name) DO UPDATE SET question=excluded.question, generated_at=excluded.generated_at, error=excluded.error`, in.Name, in.Question, now, now, runErr.Error())
	if err != nil {
		return err
	}
	_, err = s.store.db.ExecContext(ctx, `UPDATE insight_inquiries SET last_run_at = ? WHERE name = ?`, now, in.Name)
	return err
}

// RunDueInsights performs enabled automatic inquiries after calendar refresh
// cycles. Read APIs and MCP read tools never call it.
func (s *Service) RunDueInsights(ctx context.Context) {
	s.runDueInsights(ctx, true)
}

func (s *Service) runDueInsights(ctx context.Context, allowOnChange bool) {
	items, err := s.ListInsightInquiries(ctx)
	if err != nil {
		s.logger.Warn("insight schedule scan failed", "error", err)
		return
	}
	now := s.now()
	for _, item := range items {
		if !item.Enabled || item.Trigger == InsightTriggerManual {
			continue
		}
		if item.Trigger == InsightTriggerScheduled && !s.insightScheduledDue(item, now) {
			continue
		}
		if item.Trigger == InsightTriggerOnChange {
			if !allowOnChange {
				continue
			}
			changed, checkErr := s.insightSourceChanged(ctx, item)
			if checkErr != nil {
				s.logger.Warn("insight change check failed", "name", item.Name, "error", checkErr)
				continue
			}
			if !changed {
				continue
			}
		}
		if _, runErr := s.RunInsight(ctx, RunInsightInput{Name: item.Name}); runErr != nil {
			s.logger.Warn("scheduled insight failed", "name", item.Name, "error", runErr)
		}
	}
}

func validInsightTrigger(trigger InsightTrigger) bool {
	return trigger == InsightTriggerManual || trigger == InsightTriggerOnChange || trigger == InsightTriggerScheduled
}

func parseDailyInsightTime(value string) (hour, minute int, err error) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return 0, 0, fmt.Errorf("scheduled time must use HH:MM (for example 06:00)")
	}
	return parsed.Hour(), parsed.Minute(), nil
}

func (s *Service) insightScheduledDue(item InsightInquiry, now time.Time) bool {
	hour, minute, err := parseDailyInsightTime(item.Schedule)
	if err != nil {
		return false
	}
	localNow := now.In(s.location)
	due := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), hour, minute, 0, 0, s.location)
	if localNow.Before(due) {
		return false
	}
	return item.LastRunAt.IsZero() || item.LastRunAt.In(s.location).Format("2006-01-02") != localNow.Format("2006-01-02")
}

func (s *Service) insightSourceChanged(ctx context.Context, item InsightInquiry) (bool, error) {
	meetings, err := s.UpcomingMeetings(ctx, UpcomingQuery{CalendarIDs: item.CalendarIDs, Tags: item.Tags, Limit: 100, Detail: "full", IncludeDescription: false, IncludeDisabled: false})
	if err != nil {
		return false, err
	}
	events, err := json.Marshal(meetings)
	if err != nil {
		return false, err
	}
	hash := sha256.Sum256(events)
	cached, err := s.GetInsight(ctx, item.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return cached.SourceHash != hex.EncodeToString(hash[:]), nil
}
