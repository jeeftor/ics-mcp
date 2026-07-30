import * as mdi from '@mdi/js';

/** The complete local Material Design Icons catalogue. No network lookup is needed. */
const icons = Object.fromEntries(Object.entries(mdi)
  .filter(([name, value]) => name.startsWith('mdi') && typeof value === 'string')
  .map(([name, value]) => [name.slice(3).replace(/^./, letter => letter.toLowerCase()).replace(/[A-Z]/g, letter => `-${letter.toLowerCase()}`), value as string]));

export const calendarIconChoices = Object.keys(icons).sort();

function iconKey(icon: string): string { return icon.trim().toLowerCase().replace(/^mdi:/, ''); }

export function calendarColor(calendar: { color?: string; key: string }): string {
  if (calendar.color) return calendar.color;
  let hash = 0;
  for (const char of calendar.key) hash = (hash * 31 + char.charCodeAt(0)) >>> 0;
  const hue = hash % 360;
  const saturation = 58 + ((hash >>> 8) % 14);
  const lightness = 42 + ((hash >>> 16) % 9);
  return hslToHex(hue, saturation, lightness);
}

function hslToHex(hue: number, saturation: number, lightness: number): string {
  const s = saturation / 100; const l = lightness / 100; const c = (1 - Math.abs(2 * l - 1)) * s; const x = c * (1 - Math.abs((hue / 60) % 2 - 1)); const m = l - c / 2;
  const [r, g, b] = hue < 60 ? [c, x, 0] : hue < 120 ? [x, c, 0] : hue < 180 ? [0, c, x] : hue < 240 ? [0, c, x] : hue < 300 ? [x, 0, c] : [c, 0, x];
  return `#${[r, g, b].map(value => Math.round((value + m) * 255).toString(16).padStart(2, '0')).join('')}`;
}

/** Render an MDI icon, falling back safely to calendar for an unknown name. */
export function CalendarGlyph({ icon = 'calendar', color, size = 18 }: { icon?: string; color: string; size?: number }) {
  return <span className="calendar-glyph" style={{ '--calendar-color': color, width: size, height: size } as React.CSSProperties} aria-hidden="true"><svg viewBox="0 0 24 24"><path d={icons[iconKey(icon)] || icons.calendar}/></svg></span>;
}
