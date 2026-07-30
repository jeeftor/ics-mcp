import { mdiAccountSchool, mdiAirplane, mdiBriefcase, mdiCalendar, mdiCalendarHeart, mdiChurch, mdiFoodForkDrink, mdiHome, mdiStar } from '@mdi/js';

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
};

export const calendarIconChoices = Object.keys(icons);

export function calendarColor(calendar: { color?: string; key: string }): string {
  if (calendar.color) return calendar.color;
  const palette = ['#4f8f72', '#7c6ee6', '#d16f5d', '#b67c32', '#368a9b', '#aa5f88'];
  return palette[[...calendar.key].reduce((total, char) => total + char.charCodeAt(0), 0) % palette.length];
}

export function CalendarGlyph({ icon = 'calendar', color, size = 18 }: { icon?: string; color: string; size?: number }) {
  return <span className="calendar-glyph" style={{ '--calendar-color': color, width: size, height: size } as React.CSSProperties} aria-hidden="true"><svg viewBox="0 0 24 24"><path d={icons[icon] || icons.calendar}/></svg></span>;
}
