import { DateTimeField } from "./DateTimeField";
import { Scheduled, Deadline, parseDate, localInstant } from "./scheduling";
import { displayDate, displayInstant } from "./calendar";
export type ScheduleDraft = {
  kind: string;
  start: string;
  end: string;
  zone: string;
  deadlineKind: string;
  deadline: string;
  deadlineZone: string;
  offsetChoice: string;
};
export function scheduleDraft(
  s: Scheduled | null | undefined,
  d: Deadline | null | undefined,
  zone: string,
): ScheduleDraft {
  return {
    kind: s ? (s.kind ?? "timed") : "none",
    start: s
      ? s.kind === "date_range"
        ? displayDate(s.startDate)
        : displayInstant(s.startAt, s.zone)
      : "",
    end: s
      ? s.kind === "date_range"
        ? s.endDate
          ? displayDate(s.endDate)
          : ""
        : s.endAt
          ? displayInstant(s.endAt, s.zone)
          : ""
      : "",
    zone: s?.zone ?? zone,
    deadlineKind: d?.kind ?? "none",
    deadline: d
      ? d.kind === "date"
        ? displayDate(d.date)
        : displayInstant(d.at, d.zone)
      : "",
    deadlineZone: d?.zone ?? zone,
    offsetChoice: "",
  };
}
export function schedulePatch(v: ScheduleDraft): {
  scheduled: Scheduled | null;
  deadline: Deadline | null;
} {
  let scheduled: Scheduled | null = null,
    deadline: Deadline | null = null;
  // Validate the zone even for dates, without assigning an artificial time to them.
  new Intl.DateTimeFormat("en", { timeZone: v.zone });
  new Intl.DateTimeFormat("en", { timeZone: v.deadlineZone });
  if (v.kind === "date_range") {
    const startDate = parseDate(v.start),
      endDate = v.end ? parseDate(v.end) : null;
    if (endDate && endDate < startDate)
      throw new Error("Окончание раньше начала");
    scheduled = { kind: "date_range", startDate, endDate, zone: v.zone };
  }
  if (v.kind === "timed") {
    const startAt = localInstant(v.start, v.zone, v.offsetChoice),
      endAt = v.end ? localInstant(v.end, v.zone, v.offsetChoice) : null;
    if (endAt && endAt <= startAt)
      throw new Error("Конец должен быть позже начала");
    scheduled = { kind: "timed", startAt, endAt, zone: v.zone };
  }
  if (v.deadlineKind === "date")
    deadline = {
      kind: "date",
      date: parseDate(v.deadline),
      zone: v.deadlineZone,
    };
  if (v.deadlineKind === "instant")
    deadline = {
      kind: "instant",
      at: localInstant(v.deadline, v.deadlineZone, v.offsetChoice),
      zone: v.deadlineZone,
    };
  return { scheduled, deadline };
}
export function SchedulingFields({
  value,
  onChange,
}: {
  value: ScheduleDraft;
  onChange: (v: ScheduleDraft) => void;
}) {
  const set = (key: keyof ScheduleDraft, v: string) =>
    onChange({ ...value, [key]: v });
  return (
    <section id="task-planning">
      <h3>Календарные сроки</h3>
      <div className="fields">
        <label>
          Тип интервала
          <select
            value={value.kind}
            onChange={(e) => {
              onChange({ ...value, kind: e.target.value, start: "", end: "" });
            }}
          >
            <option value="none">Без дат</option>
            <option value="date_range">Целые даты</option>
            <option value="timed">Точное время</option>
          </select>
        </label>
        <label>
          Тип дедлайна
          <select
            value={value.deadlineKind}
            onChange={(e) =>
              onChange({ ...value, deadlineKind: e.target.value, deadline: "" })
            }
          >
            <option value="none">Без дедлайна</option>
            <option value="date">Дата</option>
            <option value="instant">Дата и время</option>
          </select>
        </label>
      </div>
      {value.kind !== "none" && (
        <div className="fields">
          <label>
            Начало
            <DateTimeField label="Начало" value={value.start} onChange={v => set("start", v)} withTime={value.kind === "timed"} required />
          </label>
          <label>
            {value.kind === "date_range"
              ? "Окончание, включительно"
              : "Окончание"}
            <DateTimeField label={value.kind === "date_range" ? "Окончание, включительно" : "Окончание"} value={value.end} onChange={v => set("end", v)} withTime={value.kind === "timed"} />
          </label>
          <label>
            Часовой пояс интервала
            <input
              value={value.zone}
              onChange={(e) => set("zone", e.target.value)}
              required
            />
          </label>
        </div>
      )}
      <div className="fields">
        {value.deadlineKind !== "none" && (
          <>
            <label>
              Дедлайн
              <DateTimeField label="Дедлайн" value={value.deadline} onChange={v => set("deadline", v)} withTime={value.deadlineKind === "instant"} required />
            </label>
            <label>
              Часовой пояс дедлайна
              <input
                value={value.deadlineZone}
                onChange={(e) => set("deadlineZone", e.target.value)}
                required
              />
            </label>
          </>
        )}
      </div>
      {(value.kind === "timed" || value.deadlineKind === "instant") && (
        <label>
          Неоднозначное время DST
          <select
            value={value.offsetChoice}
            onChange={(e) => set("offsetChoice", e.target.value)}
          >
            <option value="">Требовать однозначное время</option>
            <option value="earlier">Раннее вхождение</option>
            <option value="later">Позднее вхождение</option>
          </select>
        </label>
      )}
      <p>
        Пустое окончание — только начало, без выдуманной длительности. Полоса не
        означает часы работы и не меняет план дня. Дедлайн переносится отдельно.
      </p>
      {value.kind !== "none" && (
        <button type="button" onClick={() => set("kind", "none")}>
          Убрать из расписания
        </button>
      )}
    </section>
  );
}
