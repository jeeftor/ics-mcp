import { useState } from 'react';

export type JSONOutputValue = { readonly kind: 'json'; readonly value: unknown; readonly raw: string } | { readonly kind: 'text'; readonly raw: string };

/** Convert an API result without treating ordinary text as JSON. */
export function jsonOutputValue(value: unknown): JSONOutputValue {
  if (typeof value === 'string') {
    try { return { kind: 'json', value: JSON.parse(value), raw: value }; } catch { return { kind: 'text', raw: value }; }
  }
  return { kind: 'json', value, raw: JSON.stringify(value) };
}

/** Serialize structured output in compact or readable form. */
export function formatJSON(value: unknown, compact: boolean): string { return JSON.stringify(value, null, compact ? undefined : 2); }

function JSONTokens({ source }: { source: string }) {
  const parts = source.split(/("(?:\\.|[^"\\])*")(?=\s*:)|("(?:\\.|[^"\\])*")|\b(true|false)\b|\b(null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g);
  return <>{parts.map((part, index) => {
    if (!part) return null;
    const next = parts[index + 1];
    if (part.startsWith('"')) return <span className={next === undefined ? 'json-string' : 'json-key'} key={index}>{part}</span>;
    if (part === 'true' || part === 'false') return <span className="json-boolean" key={index}>{part}</span>;
    if (part === 'null') return <span className="json-null" key={index}>{part}</span>;
    if (/^-?\d/.test(part)) return <span className="json-number" key={index}>{part}</span>;
    return part;
  })}</>;
}

/** A readable result area with JSON controls and a safe plain-text fallback. */
export function JSONOutput({ value }: { value: unknown }) {
  const [compact, setCompact] = useState(false); const [copied, setCopied] = useState(false);
  if (value === undefined) return null;
  const output = jsonOutputValue(value); const text = output.kind === 'json' ? formatJSON(output.value, compact) : output.raw;
  const copy = async () => { await navigator.clipboard?.writeText(output.raw); setCopied(true); window.setTimeout(() => setCopied(false), 1600); };
  return <section className="json-output" aria-label="API output"><div className="json-output-toolbar"><span>{output.kind === 'json' ? 'JSON response' : 'Text response'}</span><div>{output.kind === 'json' && <button type="button" className="json-toggle" onClick={() => setCompact(current => !current)}>{compact ? 'Pretty' : 'Compact'}</button>}<button type="button" className="json-toggle" onClick={() => void copy()}>{copied ? 'Copied' : 'Copy raw'}</button></div></div><pre className="api-output">{output.kind === 'json' ? <JSONTokens source={text}/> : text}</pre></section>;
}
