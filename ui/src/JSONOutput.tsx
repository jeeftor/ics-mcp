import { useState } from 'react';

export type JSONOutputValue = { readonly kind: 'json'; readonly value: unknown; readonly raw: string } | { readonly kind: 'html'; readonly raw: string } | { readonly kind: 'text'; readonly raw: string };

/** Detect whether a string looks like HTML/XML markup (starts with a tag). */
function looksLikeHTML(value: string): boolean {
  const trimmed = value.trimStart();
  if (!trimmed.startsWith('<')) return false;
  // Must have a closing > and a tag name after the < — not just "<3" or "< word"
  return /^<[a-zA-Z!\/?]/.test(trimmed) && trimmed.includes('>');
}

/** Convert an API result without treating ordinary text as JSON. */
export function jsonOutputValue(value: unknown): JSONOutputValue {
  if (typeof value === 'string') {
    try { return { kind: 'json', value: JSON.parse(value), raw: value }; } catch { /* not JSON */ }
    if (looksLikeHTML(value)) return { kind: 'html', raw: value };
    return { kind: 'text', raw: value };
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

/** Highlight HTML/XML tags, attribute names, attribute values, and text content. */
function HTMLTokens({ source }: { source: string }) {
  const parts = source.split(/(<\/?[a-zA-Z][^>]*>|<!--[\s\S]*?-->|<\/?>)/g);
  return <>{parts.map((part, index) => {
    if (!part) return null;
    if (part.startsWith('<!--')) return <span className="html-comment" key={index}>{part}</span>;
    if (part.startsWith('<')) {
      // Split the tag into the bracket, tag name, attributes, and closing bracket
      const tagParts = part.split(/(<\/?|[a-zA-Z][a-zA-Z0-9-]*|"[^"]*"|'[^']*'|\s+|\/?>)/g);
      return <span className="html-tag" key={index}>{tagParts.map((tp, ti) => {
        if (!tp) return null;
        if (tp === '</' || tp === '<' || tp === '/>' || tp === '>') return <span className="html-bracket" key={ti}>{tp}</span>;
        if (/^[a-zA-Z]/.test(tp)) {
          // First identifier after < is the tag name; subsequent ones are attribute names
          const prevNonSpace = tagParts.slice(0, ti).reverse().find(p => p && p.trim());
          if (prevNonSpace === '<' || prevNonSpace === '</') return <span className="html-tag-name" key={ti}>{tp}</span>;
          return <span className="html-attr-name" key={ti}>{tp}</span>;
        }
        if (tp.startsWith('"') || tp.startsWith("'")) return <span className="html-attr-value" key={ti}>{tp}</span>;
        return tp;
      })}</span>;
    }
    return part;
  })}</>;
}

/** A readable result area with JSON/HTML controls and a safe plain-text fallback. */
export function JSONOutput({ value }: { value: unknown }) {
  const [compact, setCompact] = useState(false); const [copied, setCopied] = useState(false);
  if (value === undefined) return null;
  const output = jsonOutputValue(value);
  const text = output.kind === 'json' ? formatJSON(output.value, compact) : output.raw;
  const label = output.kind === 'json' ? 'JSON response' : output.kind === 'html' ? 'HTML response' : 'Text response';
  const copy = async () => { await navigator.clipboard?.writeText(output.raw); setCopied(true); window.setTimeout(() => setCopied(false), 1600); };
  return <section className="json-output" aria-label="API output"><div className="json-output-toolbar"><span>{label}</span><div>{output.kind === 'json' && <button type="button" className="json-toggle" onClick={() => setCompact(current => !current)}>{compact ? 'Pretty' : 'Compact'}</button>}<button type="button" className="json-toggle" onClick={() => void copy()}>{copied ? 'Copied' : 'Copy raw'}</button></div></div><pre className="api-output">{output.kind === 'json' ? <JSONTokens source={text}/> : output.kind === 'html' ? <HTMLTokens source={text}/> : text}</pre></section>;
}
