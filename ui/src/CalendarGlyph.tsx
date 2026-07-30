import * as mdi from '@mdi/js';

/** The complete local Material Design Icons catalogue. No network lookup is needed. */
const icons = Object.fromEntries(Object.entries(mdi)
  .filter(([name, value]) => name.startsWith('mdi') && typeof value === 'string')
  .map(([name, value]) => [name.slice(3).replace(/^./, letter => letter.toLowerCase()).replace(/[A-Z]/g, letter => `-${letter.toLowerCase()}`), value as string]));

export const calendarIconChoices = Object.keys(icons).sort();

/** A deliberately small, high-contrast set for calendar and tag presentation. */
export const calendarColorPalette = [
  '#2f7d5a', '#3777b8', '#8b5bb9', '#c13f77',
  '#bf6b21', '#147d8d', '#73823c', '#b64d35',
] as const;

function iconKey(icon: string): string { return icon.trim().toLowerCase().replace(/^mdi:/, ''); }

export function calendarColor(calendar: { color?: string; key: string; tags?: string[] }, tags: Array<{ name: string; color?: string }> = []): string {
  if (calendar.color) return calendar.color;
  const inherited = calendar.tags?.map(name => tags.find(tag => tag.name === name)?.color).find(Boolean);
  return inherited || paletteColor(calendar.key);
}

function paletteIndex(value: string): number {
  let hash = 0;
  for (const char of value) hash = (hash * 31 + char.charCodeAt(0)) >>> 0;
  return hash % calendarColorPalette.length;
}

/** Return a stable palette color when no explicit presentation color was saved. */
export function paletteColor(seed: string): string {
  return calendarColorPalette[paletteIndex(seed)];
}

/** Pick a distinct palette color where one remains, without overwriting saved colors. */
export function nextPaletteColor(existing: Iterable<string | undefined>, seed: string): string {
  const used = new Set([...existing].filter((color): color is string => Boolean(color)).map(color => color.toLowerCase()));
  const start = paletteIndex(seed);
  for (let offset = 0; offset < calendarColorPalette.length; offset += 1) {
    const color = calendarColorPalette[(start + offset) % calendarColorPalette.length];
    if (!used.has(color.toLowerCase())) return color;
  }
  return calendarColorPalette[start];
}

/** Render an MDI icon, falling back safely to calendar for an unknown name. */
export function CalendarGlyph({ icon = 'calendar', color, size = 18 }: { icon?: string; color: string; size?: number }) {
  return <span className="calendar-glyph" style={{ '--calendar-color': color, width: size, height: size } as React.CSSProperties} aria-hidden="true"><svg viewBox="0 0 24 24"><path d={icons[iconKey(icon)] || icons.calendar}/></svg></span>;
}
