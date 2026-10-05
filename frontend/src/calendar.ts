/** Calendar arithmetic deliberately operates on date labels, not local 24-hour durations. */
export function dayNumber(date: string): number {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) throw new Error("Неверная дата");
  const ms = Date.parse(date + "T00:00:00Z");
  if (!Number.isFinite(ms) || new Date(ms).toISOString().slice(0, 10) !== date)
    throw new Error("Неверная дата");
  return ms / 86400000;
}
export function addDays(date: string, days: number): string {
  if (!Number.isInteger(days)) throw new Error("Неверное число дней");
  return new Date((dayNumber(date) + days) * 86400000)
    .toISOString()
    .slice(0, 10);
}
export function displayDate(date: string): string {
  dayNumber(date);
  return date.replaceAll("-", ".");
}
export function displayInstant(instant: string, zone: string): string {
  const parts = new Intl.DateTimeFormat("en-GB", {
    timeZone: zone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
  }).formatToParts(new Date(instant));
  const v = (name: string) => parts.find((p) => p.type === name)?.value;
  return `${v("year")}.${v("month")}.${v("day")} ${v("hour")}:${v("minute")}:${v("second")}`;
}
export function dateInZone(instant: string, zone: string): string {
  return displayInstant(instant, zone).slice(0, 10).replaceAll(".", "-");
}
/** Clip only the drawing; callers retain the original dates for editing. */
export function visibleInterval(
  start: string,
  end: string,
  viewStart: string,
  days: number,
): { left: number; width: number } | null {
  const a = dayNumber(start),
    b = dayNumber(end),
    v = dayNumber(viewStart);
  if (b < a || !Number.isInteger(days) || days < 1)
    throw new Error("Неверный интервал");
  const left = Math.max(a, v),
    right = Math.min(b, v + days - 1);
  return right < left
    ? null
    : {
        left: ((left - v) / days) * 100,
        width: ((right - left + 1) / days) * 100,
      };
}
