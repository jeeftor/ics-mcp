import { useId, useMemo, useRef, useState } from 'react';

export type TagSuggestion = { name: string };

export function activeTagFragment(value: string, cursor: number): { start: number; end: number; query: string } {
  const start = value.lastIndexOf(',', Math.max(0, cursor - 1)) + 1;
  const nextComma = value.indexOf(',', cursor);
  const end = nextComma === -1 ? value.length : nextComma;
  return { start, end, query: value.slice(start, end).trim() };
}

export function matchingTags(value: string, cursor: number, tags: TagSuggestion[]): TagSuggestion[] {
  const query = activeTagFragment(value, cursor).query.toLocaleLowerCase();
  if (!query) return [];
  return tags.filter(tag => tag.name.toLocaleLowerCase().includes(query) && tag.name.localeCompare(query, undefined, { sensitivity: 'accent' }) !== 0);
}

export function replaceActiveTag(value: string, cursor: number, tag: string): { value: string; cursor: number } {
  const { start, end } = activeTagFragment(value, cursor);
  const leadingSpace = value.slice(start, end).match(/^\s*/)?.[0] || '';
  const nextValue = `${value.slice(0, start)}${leadingSpace}${tag}${value.slice(end)}`;
  return { value: nextValue, cursor: start + leadingSpace.length + tag.length };
}

export function TagAutocomplete({ value, tags, onChange }: { value: string; tags: TagSuggestion[]; onChange: (value: string) => void }) {
  const inputRef = useRef<HTMLInputElement>(null);
  const listboxID = useId();
  const [focused, setFocused] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const cursor = inputRef.current?.selectionStart ?? value.length;
  const suggestions = useMemo(() => matchingTags(value, cursor, tags), [value, cursor, tags]);
  const open = focused && suggestions.length > 0;
  const choose = (tag: string) => {
    const replacement = replaceActiveTag(value, inputRef.current?.selectionStart ?? value.length, tag);
    onChange(replacement.value);
    requestAnimationFrame(() => {
      inputRef.current?.focus();
      inputRef.current?.setSelectionRange(replacement.cursor, replacement.cursor);
    });
  };
  const onKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (!open) return;
    if (event.key === 'ArrowDown') { event.preventDefault(); setActiveIndex(index => (index + 1) % suggestions.length); }
    else if (event.key === 'ArrowUp') { event.preventDefault(); setActiveIndex(index => (index - 1 + suggestions.length) % suggestions.length); }
    else if (event.key === 'Enter' || event.key === 'Tab') { event.preventDefault(); choose(suggestions[activeIndex].name); }
    else if (event.key === 'Escape') { event.preventDefault(); setFocused(false); }
  };
  return <div style={{ position: 'relative' }}><input ref={inputRef} value={value} onChange={event => { onChange(event.target.value); setActiveIndex(0); }} onFocus={() => setFocused(true)} onBlur={() => setFocused(false)} onKeyDown={onKeyDown} role="combobox" aria-autocomplete="list" aria-expanded={open} aria-controls={open ? listboxID : undefined} aria-activedescendant={open ? `${listboxID}-${activeIndex}` : undefined}/>{open && <div id={listboxID} role="listbox" aria-label="Matching tags" style={{ position: 'absolute', zIndex: 20, top: 'calc(100% + 4px)', right: 0, left: 0, display: 'grid', maxHeight: 180, overflowY: 'auto', border: '1px solid #d6d1c7', borderRadius: 8, background: '#fffefa', boxShadow: '0 10px 24px rgba(55,48,38,.18)' }}>{suggestions.map((tag, index) => <button type="button" id={`${listboxID}-${index}`} role="option" aria-selected={index === activeIndex} key={tag.name} style={{ width: '100%', border: 0, borderTop: index ? '1px solid #eeeae3' : 0, padding: '8px 10px', color: '#474138', background: index === activeIndex ? '#edf4eb' : 'transparent', boxShadow: index === activeIndex ? 'inset 3px 0 #5d8966' : undefined, font: 'inherit', fontSize: 12, textAlign: 'left', cursor: 'pointer' }} onMouseDown={event => { event.preventDefault(); choose(tag.name); }}>{tag.name}</button>)}</div>}</div>;
}
