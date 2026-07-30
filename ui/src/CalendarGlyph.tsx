import { mdiAccount, mdiAccountSchool, mdiAirplane, mdiAlarm, mdiBasketball, mdiBookOpenVariant, mdiBriefcase, mdiCalendar, mdiCalendarHeart, mdiCamera, mdiCar, mdiChurch, mdiDumbbell, mdiFoodForkDrink, mdiHome, mdiLaptop, mdiPaw, mdiStar, mdiWalletTravel } from '@mdi/js';

const icons: Record<string, string> = {
  calendar: mdiCalendar,
  work: mdiBriefcase,
  school: mdiAccountSchool,
  home: mdiHome,
  personal: mdiCalendarHeart,
  holiday: mdiAirplane,
  food: mdiFoodForkDrink,
  church: mdiChurch,
  star: mdiStar
  , account: mdiAccount, alarm: mdiAlarm, basketball: mdiBasketball, book: mdiBookOpenVariant, camera: mdiCamera, car: mdiCar, fitness: mdiDumbbell, laptop: mdiLaptop, pet: mdiPaw, travel: mdiWalletTravel
};

export const calendarIconChoices = Object.keys(icons);

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
  const [r, g, b] = hue < 60 ? [c, x, 0] : hue < 120 ? [x, c, 0] : hue < 180 ? [0, c, x] : hue < 240 ? [0, x, c] : hue < 300 ? [x, 0, c] : [c, 0, x];
  return `#${[r, g, b].map(value => Math.round((value + m) * 255).toString(16).padStart(2, '0')).join('')}`;
}

export function CalendarGlyph({ icon = 'calendar', color, size = 18 }: { icon?: string; color: string; size?: number }) {
  return <span className="calendar-glyph" style={{ '--calendar-color': color, width: size, height: size } as React.CSSProperties} aria-hidden="true"><svg viewBox="0 0 24 24"><path d={icons[iconKey(icon)] || icons.calendar}/></svg></span>;
}
