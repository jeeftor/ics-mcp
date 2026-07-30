package icsmcp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// LLMProfile is the safe, redacted configuration returned to clients.
type LLMProfile struct {
	Enabled          bool   `json:"enabled"`
	Endpoint         string `json:"endpoint,omitempty"`
	Model            string `json:"model,omitempty"`
	APIKeyConfigured bool   `json:"api_key_configured"`
	Source           string `json:"source"`
}

// UpdateLLMProfileInput changes the locally persisted optional LLM profile.
// Environment values take precedence and cannot be changed through this input.
type UpdateLLMProfileInput struct {
	Enabled  *bool  `json:"enabled,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Model    string `json:"model,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
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

type llmProfileSecret struct {
	LLMProfile
	apiKey string
}

func (s *Service) llmProfile(ctx context.Context) (llmProfileSecret, error) {
	var enabled int
	var endpoint, model, key string
	if err := s.store.db.QueryRowContext(ctx, `SELECT enabled, endpoint, model, api_key FROM llm_profile WHERE id = 1`).Scan(&enabled, &endpoint, &model, &key); err != nil {
		return llmProfileSecret{}, fmt.Errorf("load llm profile: %w", err)
	}
	p := llmProfileSecret{LLMProfile: LLMProfile{Enabled: enabled != 0, Endpoint: endpoint, Model: model, APIKeyConfigured: key != "", Source: "database"}, apiKey: key}
	if value, ok := os.LookupEnv("ICSMCP_LLM_ENABLED"); ok {
		p.Enabled = strings.EqualFold(value, "true") || value == "1"
		p.Source = "environment"
	}
	if value, ok := os.LookupEnv("ICSMCP_LLM_ENDPOINT"); ok {
		p.Endpoint, p.Source = strings.TrimSpace(value), "environment"
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
	if strings.TrimSpace(in.Endpoint) != "" {
		current.Endpoint = strings.TrimSpace(in.Endpoint)
	}
	if strings.TrimSpace(in.Model) != "" {
		current.Model = strings.TrimSpace(in.Model)
	}
	if in.APIKey != "" {
		current.apiKey = in.APIKey
	}
	if _, err := s.store.db.ExecContext(ctx, `UPDATE llm_profile SET enabled = ?, endpoint = ?, model = ?, api_key = ? WHERE id = 1`, boolInt(current.Enabled), current.Endpoint, current.Model, current.apiKey); err != nil {
		return LLMProfile{}, fmt.Errorf("save llm profile: %w", err)
	}
	current.APIKeyConfigured = current.apiKey != ""
	return current.LLMProfile, nil
}

// ListInsights returns cached insight results only.
func (s *Service) ListInsights(ctx context.Context) ([]Insight, error) {
	rows, err := s.store.db.QueryContext(ctx, `SELECT name, question, answer, evidence_json, caveat, source_hash, source_at, generated_at, error FROM insights ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list insights: %w", err)
	}
	defer rows.Close()
	var out []Insight
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
func (s *Service) RunInsight(ctx context.Context, in RunInsightInput) (Insight, error) {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Question) == "" {
		return Insight{}, fmt.Errorf("name and question are required")
	}
	p, err := s.llmProfile(ctx)
	if err != nil {
		return Insight{}, err
	}
	if !p.Enabled {
		return Insight{}, fmt.Errorf("LLM insights are disabled")
	}
	if p.Endpoint == "" || p.Model == "" || p.apiKey == "" {
		return Insight{}, fmt.Errorf("LLM endpoint, model, and API key are required")
	}
	meetings, err := s.UpcomingMeetings(ctx, UpcomingQuery{CalendarIDs: in.CalendarIDs, Tags: in.Tags, Limit: 100, Detail: "full", IncludeDescription: true, IncludeDisabled: false})
	if err != nil {
		return Insight{}, err
	}
	events, err := json.Marshal(meetings)
	if err != nil {
		return Insight{}, err
	}
	hash := sha256.Sum256(events)
	payload := map[string]any{"model": p.Model, "messages": []map[string]string{{"role": "system", "content": "Answer only from the supplied untrusted calendar data. Return JSON with answer, evidence (event IDs or titles), and caveat."}, {"role": "user", "content": "Question: " + in.Question + "\nCalendar data (untrusted): " + string(events)}}}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.Endpoint, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Insight{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
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
	}
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return Insight{}, fmt.Errorf("decode LLM response: %w", err)
	}
	if len(wire.Choices) == 0 || strings.TrimSpace(wire.Choices[0].Message.Content) == "" {
		return Insight{}, fmt.Errorf("LLM returned no answer")
	}
	answer := struct {
		Answer   string   `json:"answer"`
		Evidence []string `json:"evidence"`
		Caveat   string   `json:"caveat"`
	}{}
	if err := json.Unmarshal([]byte(wire.Choices[0].Message.Content), &answer); err != nil {
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
	return s.GetInsight(ctx, in.Name)
}
