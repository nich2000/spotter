import { addDays, dayNumber, dateInZone, displayInstant } from "./calendar";
import { scheduleDates, deadlineDate } from "./scheduling";
import type { Task } from "./api";
export type CalendarEntry = {
  id: string;
  title: string;
  startAt: string;
  endAt: string;
  kind: "work" | "break" | "external" | "task";
  taskId?: string;
  source?: string;
  allDay?: boolean;
  pinned?: boolean;
};
export function monthDays(month: string) {
  const first = month.slice(0, 7) + "-01";
  const d = new Date(first + "T00:00:00Z");
  const offset = (d.getUTCDay() + 6) % 7;
  const next = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth() + 1, 1))
    .toISOString()
    .slice(0, 10);
  return Array.from(
    {
      length: Math.ceil((dayNumber(next) - dayNumber(first) + offset) / 7) * 7,
    },
    (_, i) => addDays(first, i - offset),
  );
}
export function shiftMonth(month: string, delta: number) {
  const d = new Date(month.slice(0, 7) + "-01T00:00:00Z");
  return new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth() + delta, 1))
    .toISOString()
    .slice(0, 10);
}
export function onDay(task: Task, date: string) {
  const s = task.scheduled;
  if (!s) return false;
  const [a, b] = scheduleDates(s);
  return date >= a && date <= (b ?? a);
}
export function dueOn(task: Task, date: string) {
  return task.deadline && deadlineDate(task.deadline) === date;
}
export function dayEntries(
  entries: CalendarEntry[],
  date: string,
  zone: string,
) {
  return entries
    .filter(
      (e) =>
        dateInZone(e.startAt, zone) <= date &&
        dateInZone(
          new Date(
            Math.max(Date.parse(e.startAt), Date.parse(e.endAt) - 1),
          ).toISOString(),
          zone,
        ) >= date,
    )
    .sort((a, b) => a.startAt.localeCompare(b.startAt));
}
export function eventMinutes(e: CalendarEntry, date: string, zone: string) {
  const minute = (v: string) => {
    const t = displayInstant(v, zone).slice(11).split(":").map(Number);
    return t[0] * 60 + t[1] + t[2] / 60;
  };
  return {
    start: dateInZone(e.startAt, zone) < date ? 0 : minute(e.startAt),
    end: dateInZone(e.endAt, zone) > date ? 1440 : minute(e.endAt),
  };
}
/** Connected overlap groups receive parallel lanes, retaining half-open boundaries. */
export function layoutEntries(
  entries: CalendarEntry[],
  date: string,
  zone: string,
) {
  const rows = dayEntries(entries, date, zone)
    .filter((e) => !e.allDay)
    .map((e) => ({ ...e, ...eventMinutes(e, date, zone), lane: 0, lanes: 1 }))
    .sort((a, b) => a.start - b.start);
  let group: typeof rows = [],
    ends: number[] = [],
    until = -1;
  const flush = () => {
    group.forEach((e) => (e.lanes = ends.length));
    group = [];
    ends = [];
  };
  for (const row of rows) {
    if (row.start >= until) {
      flush();
      until = -1;
    }
    let lane = ends.findIndex((end) => end <= row.start);
    if (lane < 0) lane = ends.length;
    row.lane = lane;
    ends[lane] = row.end;
    until = Math.max(until, row.end);
    group.push(row);
  }
  flush();
  return rows;
}
export function weekRibbons(tasks: Task[], week: string[]) {
  const ends: number[] = [];
  return tasks.flatMap((t) => {
    if (t.scheduled?.kind !== "date_range") return [];
    const [a, b] = scheduleDates(t.scheduled);
    const start = Math.max(0, dayNumber(a) - dayNumber(week[0])),
      end = Math.min(6, dayNumber(b ?? a) - dayNumber(week[0]));
    if (start > end) return [];
    let lane = ends.findIndex((e) => e < start);
    if (lane < 0) lane = ends.length;
    ends[lane] = end;
    return [{ task: t, start, end, lane }];
  });
}
