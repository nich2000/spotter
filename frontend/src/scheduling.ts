import { addDays, displayInstant, dayNumber, dateInZone } from "./calendar";
export type Scheduled =
  | {
      kind: "date_range";
      startDate: string;
      endDate: string | null;
      zone: string;
    }
  | {
      kind: "timed";
      startAt: string;
      endAt: string | null;
      zone: string;
      localStart?: string;
      localEnd?: string | null;
      offsetChoice?: "earlier" | "later" | null;
    };
export type Deadline =
  | { kind: "date"; date: string; zone: string }
  | { kind: "instant"; at: string; zone: string };
export function parseDate(text: string) {
  const date = text.trim().replaceAll(".", "-");
  dayNumber(date);
  return date;
}
export function localInstant(
  text: string,
  zone: string,
  choice?: string,
): string {
  const local = text.trim().replaceAll(".", "-").replace(" ", "T");
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$/.test(local))
    throw new Error("Формат: yyyy.mm.dd hh:mm:ss");
  const base = Date.parse(local + "Z");
  if (
    !Number.isFinite(base) ||
    new Date(base).toISOString().slice(0, 19) !== local
  )
    throw new Error("Неверная дата или время");
  const offsets = new Set<number>();
  for (let h = -48; h <= 48; h += 6) {
    const instant = base + h * 3600000;
    const wall = displayInstant(new Date(instant).toISOString(), zone)
      .replaceAll(".", "-")
      .replace(" ", "T");
    offsets.add(Date.parse(wall + "Z") - instant);
  }
  const target = local.replace("T", " ").replaceAll("-", ".");
  const matches = [...offsets]
    .map((o) => new Date(base - o).toISOString())
    .filter((at) => displayInstant(at, zone) === target)
    .sort();
  if (!matches.length)
    throw new Error("Такого местного времени нет из-за перехода DST");
  if (matches.length > 1 && !["earlier", "later"].includes(choice ?? ""))
    throw new Error(
      "Время неоднозначно из-за DST: выберите раннее или позднее",
    );
  return choice === "later" ? matches[matches.length - 1] : matches[0];
}
export function scheduleDates(s: Scheduled): [string, string | null] {
  return s.kind === "date_range"
    ? [s.startDate, s.endDate]
    : [
        dateInZone(s.startAt, s.zone),
        s.endAt
          ? dateInZone(new Date(Date.parse(s.endAt) - 1).toISOString(), s.zone)
          : null,
      ];
}
export function moveSchedule(
  s: Scheduled,
  delta: number,
  resize = false,
): Scheduled {
  if (s.kind === "date_range") {
    if (resize) {
      const end = addDays(s.endDate ?? s.startDate, delta);
      if (end < s.startDate) throw new Error("Окончание раньше начала");
      return { ...s, endDate: end };
    }
    return {
      ...s,
      startDate: addDays(s.startDate, delta),
      endDate: s.endDate ? addDays(s.endDate, delta) : null,
    };
  }
  const move = (at: string) => {
    const local = displayInstant(at, s.zone);
    return localInstant(
      addDays(local.slice(0, 10).replaceAll(".", "-"), delta).replaceAll(
        "-",
        ".",
      ) + local.slice(10),
      s.zone,
    );
  };
  const startAt = resize ? s.startAt : move(s.startAt),
    endAt = s.endAt ? move(s.endAt) : null;
  if (endAt && Date.parse(endAt) <= Date.parse(startAt))
    throw new Error("Конец должен быть позже начала");
  return {
    kind: "timed",
    zone: s.zone,
    startAt,
    endAt,
  };
}
export function deadlineDate(d: Deadline) {
  return d.kind === "date" ? d.date : dateInZone(d.at, d.zone);
}
/** Compare exclusive next-midnight boundaries for inclusive whole-date values. */
export function scheduleExceedsDeadline(s: Scheduled, d: Deadline): boolean {
  const boundary = (date: string, zone: string) =>
    localInstant(
      addDays(date, 1).replaceAll("-", ".") + " 00:00:00",
      zone,
      "later",
    );
  try {
    const end =
      s.kind === "date_range"
        ? boundary(s.endDate ?? s.startDate, s.zone)
        : (s.endAt ?? s.startAt);
    const limit = d.kind === "date" ? boundary(d.date, d.zone) : d.at;
    return Date.parse(end) > Date.parse(limit);
  } catch {
    return false;
  }
}
