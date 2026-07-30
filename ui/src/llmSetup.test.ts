import { describe, expect, it } from 'vitest';
import { llmConnectionError, llmPreset, shouldReplaceLLMEndpoint } from './App';

describe('LLM setup flow', () => {
  it('only replaces an empty or prior preset default server URL', () => {
    expect(shouldReplaceLLMEndpoint('', 'openai', 'ollama')).toBe(true);
    expect(shouldReplaceLLMEndpoint(llmPreset('ollama').defaultEndpoint, 'ollama', 'lemonade')).toBe(true);
    expect(shouldReplaceLLMEndpoint('http://192.168.1.91:13305', 'ollama', 'lemonade')).toBe(false);
  });

  it('explains why a localhost Ollama default can fail', () => {
    expect(llmConnectionError(new Error('Get "http://localhost:11434/api/tags": dial tcp [::1]:11434: connect: connection refused')))
      .toContain('machine running ICS MCP');
  });
});
