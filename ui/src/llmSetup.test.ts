import { describe, expect, it } from 'vitest';
import { inquiryDateScopeOptions, inquiryHistoryEndpoint, inquiryOutputEndpoint, inquiryPreviewInput, llmConnectionError, llmPreset, llmProfileFormValues, shouldReplaceLLMEndpoint } from './App';

describe('LLM setup flow', () => {
  it('only replaces an empty or prior preset default server URL', () => {
    expect(shouldReplaceLLMEndpoint('', 'openai', 'ollama')).toBe(true);
    expect(shouldReplaceLLMEndpoint(llmPreset('ollama').defaultEndpoint, 'ollama', 'lemonade')).toBe(true);
    expect(shouldReplaceLLMEndpoint('https://custom.example.test:13305', 'ollama', 'lemonade')).toBe(false);
  });

  it('explains why a localhost Ollama default can fail', () => {
    expect(llmConnectionError(new Error('Get "http://localhost:11434/api/tags": dial tcp [::1]:11434: connect: connection refused')))
      .toContain('LLM configuration or reachability problem');
  });

  it('labels safe no-header timeouts as configuration or reachability failures', () => {
    const message = llmConnectionError(new Error(JSON.stringify({ error: 'LLM server did not respond before the request timed out; no response headers were received. Check that the server is running and reachable from ICS MCP, then test the server connection again.' })));
    expect(message).toContain('LLM configuration or reachability problem');
    expect(message).not.toContain('192.168.1.91');
  });

  it('labels malformed LLM output as a model answer failure', () => {
    expect(llmConnectionError(new Error(JSON.stringify({ error: 'LLM answer must be JSON.' })))).toContain('Model answer problem');
  });

  it('uses a safe generic example for OpenAI-compatible servers', () => {
    expect(llmPreset('openai').example).toBe('https://llm.example.test/v1');
  });

  it('keeps the LLM page renderable when a stored profile omits editable fields', () => {
    expect(llmProfileFormValues({ enabled: false })).toEqual({ enabled: false, backend: 'openai', endpoint: '', model: '' });
  });

  it('exposes encoded cache-only output and history URLs for saved inquiries', () => {
    expect(inquiryOutputEndpoint('school tomorrow')).toBe('/api/v1/prompts/school%20tomorrow/output');
    expect(inquiryHistoryEndpoint('school tomorrow')).toBe('/api/v1/prompts/school%20tomorrow/history');
  });

  it('tests only the unsaved question and scope, never a save or run payload', () => {
    expect(inquiryPreviewInput({ name: 'daily_briefing', question: ' What should I know? ', calendar_ids: ['emily'], tags: ['Personal'], date_scope: 'custom', start_date: '2026-07-30', end_date: '2026-08-01', trigger: 'scheduled', schedule_mode: 'repeat', repeat_interval: '1h', enabled: false }))
      .toEqual({ question: 'What should I know?', calendar_ids: ['emily'], tags: ['Personal'], date_scope: 'custom', start_date: '2026-07-30', end_date: '2026-08-01' });
  });

  it('offers bounded date choices and only reveals a range through Custom', () => {
    expect(inquiryDateScopeOptions).toEqual([['today', 'Today'], ['tomorrow', 'Tomorrow'], ['this_week', 'This week'], ['next_7_days', 'Next 7 days'], ['all', 'All upcoming'], ['custom', 'Custom range']]);
  });
});
