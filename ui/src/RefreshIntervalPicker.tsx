import { useEffect, useId, useState } from 'react';

export const refreshIntervalChoices = [
  ['5m', '5 minutes'], ['15m', '15 minutes'], ['30m', '30 minutes'], ['1h', '1 hour'], ['6h', '6 hours'], ['12h', '12 hours'], ['24h', '24 hours'],
] as const;

type RefreshIntervalChoice = (typeof refreshIntervalChoices)[number][0];

function durationSeconds(value: string): number | undefined {
  const text = value.trim();
  if (!text) return undefined;
  const parts = [...text.matchAll(/(\d+(?:\.\d+)?)(ms|s|m|h)/g)];
  if (!parts.length || parts.map(part => part[0]).join('') !== text) return undefined;
  return parts.reduce((seconds, part) => seconds + Number(part[1]) * ({ ms: 0.001, s: 1, m: 60, h: 3600 }[part[2]] || 0), 0);
}

/** Returns the quick choice equivalent to a Go duration, if one is available. */
export function refreshIntervalChoice(value: string): RefreshIntervalChoice | undefined {
  const seconds = durationSeconds(value);
  return refreshIntervalChoices.find(([choice]) => durationSeconds(choice) === seconds)?.[0];
}

/** Shows common feed refresh values while preserving an arbitrary configured duration. */
export function RefreshIntervalPicker({ value, onChange, disabled = false, allowInherit = false }: { value: string; onChange: (value: string) => void; disabled?: boolean; allowInherit?: boolean }) {
  const id = useId();
  const currentChoice = refreshIntervalChoice(value);
  const [custom, setCustom] = useState(!currentChoice);
  useEffect(() => setCustom(!refreshIntervalChoice(value)), [value]);
  const select = (choice: RefreshIntervalChoice) => { setCustom(false); onChange(choice); };
  return <div className="refresh-interval-picker" aria-labelledby={`${id}-label`}>
    <span id={`${id}-label`} className="refresh-interval-label">Choose refresh frequency</span>
    <div className="refresh-interval-choices" role="radiogroup" aria-label="Refresh frequency">
      {refreshIntervalChoices.map(([choice, label]) => <button type="button" key={choice} role="radio" aria-checked={!custom && currentChoice === choice} className={!custom && currentChoice === choice ? 'active' : ''} disabled={disabled} onClick={() => select(choice)}>{choice}<small>{label}</small></button>)}
      <button type="button" role="radio" aria-checked={custom} className={custom ? 'active' : ''} disabled={disabled} onClick={() => setCustom(true)}>Custom</button>
    </div>
    {custom && <label className="refresh-interval-custom" htmlFor={`${id}-custom`}>{allowInherit ? 'Custom duration or leave blank to inherit' : 'Custom duration'}<input id={`${id}-custom`} value={value} disabled={disabled} onChange={event => onChange(event.target.value)} placeholder={allowInherit ? 'Inherit default, or e.g. 45m' : 'e.g. 45m'}/></label>}
  </div>;
}
