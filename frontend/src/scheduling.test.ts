import { it, expect } from "vitest";
import {
  localInstant,
  scheduleExceedsDeadline,
  moveSchedule,
  scheduleDates,
  deadlineDate,
  parseDate,
  type Scheduled,
} from "./scheduling";
import { scheduleDraft, schedulePatch } from "./SchedulingFields";
it("resolves real timezone instants and rejects DST gaps and ambiguous times", () => {
  expect(localInstant("2026.10.05 13:00:00", "Europe/Moscow")).toBe(
    "2026-10-05T10:00:00.000Z",
  );
  expect(() => localInstant("2026.03.29 02:30:00", "Europe/Berlin")).toThrow(
    "нет",
  );
  expect(() => localInstant("2026.10.25 02:30:00", "Europe/Berlin")).toThrow(
    "неоднозначно",
  );
  expect(localInstant("2026.10.25 02:30:00", "Europe/Berlin", "earlier")).toBe(
    "2026-10-25T00:30:00.000Z",
  );
  expect(localInstant("2026.10.25 02:30:00", "Europe/Berlin", "later")).toBe(
    "2026-10-25T01:30:00.000Z",
  );
  expect(() => localInstant("2026.10.05", "UTC")).toThrow("Формат");
  expect(() => localInstant("2026.02.30 25:00:00", "UTC")).toThrow("Неверная");
  expect(() => parseDate("2026.02.30")).toThrow();
});
it("moves inclusive ranges and start markers without manufacturing duration", () => {
  const s: Scheduled = {
    kind: "date_range",
    startDate: "2026-12-31",
    endDate: "2027-01-02",
    zone: "UTC",
  };
  expect(moveSchedule(s, 1)).toEqual({
    ...s,
    startDate: "2027-01-01",
    endDate: "2027-01-03",
  });
  expect(moveSchedule(s, -2, true)).toEqual({ ...s, endDate: "2026-12-31" });
  expect(() => moveSchedule(s, -3, true)).toThrow();
  expect(moveSchedule({ ...s, endDate: null }, 1)).toEqual({
    ...s,
    startDate: "2027-01-01",
    endDate: null,
  });
  expect(moveSchedule({ ...s, endDate: null }, 1, true)).toEqual({
    ...s,
    endDate: "2027-01-01",
  });
  expect(scheduleDates(s)).toEqual(["2026-12-31", "2027-01-02"]);
});
it("timed moves retain wall clock across DST and end resize leaves start untouched", () => {
  const s: Scheduled = {
    kind: "timed",
    startAt: "2026-03-28T09:00:00Z",
    endAt: "2026-03-28T11:00:00Z",
    zone: "Europe/Berlin",
  };
  const moved = moveSchedule(s, 1);
  expect(moved).toMatchObject({
    startAt: "2026-03-29T08:00:00.000Z",
    endAt: "2026-03-29T10:00:00.000Z",
  });
  expect(moveSchedule(s, 1, true)).toMatchObject({
    startAt: s.startAt,
    endAt: "2026-03-29T10:00:00.000Z",
  });
  expect(() => moveSchedule(s, -1, true)).toThrow("позже");
  expect(moveSchedule({ ...s, endAt: null }, 1)).toMatchObject({
    endAt: null,
  });
  expect(scheduleDates({ ...s, endAt: null })).toEqual(["2026-03-28", null]);
  expect(deadlineDate({ kind: "date", date: "2026-10-05", zone: "UTC" })).toBe(
    "2026-10-05",
  );
  expect(
    deadlineDate({
      kind: "instant",
      at: "2026-10-05T23:00:00Z",
      zone: "Europe/Moscow",
    }),
  ).toBe("2026-10-06");
});
it("roundtrips date and timed drafts, clears only requested fields and validates order", () => {
  let v = scheduleDraft(null, null, "UTC");
  expect(schedulePatch(v)).toEqual({ scheduled: null, deadline: null });
  v = {
    ...v,
    kind: "date_range",
    start: "2026.10.05",
    end: "2026.10.07",
    deadlineKind: "date",
    deadline: "2026.10.06",
  };
  const p = schedulePatch(v);
  expect(scheduleDraft(p.scheduled, p.deadline, "UTC")).toEqual(v);
  expect(schedulePatch({ ...v, end: "" }).scheduled).toMatchObject({
    endDate: null,
  });
  expect(() => schedulePatch({ ...v, end: "2026.10.04" })).toThrow();
  v = {
    ...v,
    kind: "timed",
    start: "2026.10.05 10:00:00",
    end: "2026.10.05 11:00:00",
    deadlineKind: "instant",
    deadline: "2026.10.05 12:00:00",
  };
  const q = schedulePatch(v);
  expect(scheduleDraft(q.scheduled, q.deadline, "UTC")).toEqual(v);
  expect(
    scheduleDraft({ ...q.scheduled!, kind: undefined } as any, null, "UTC")
      .kind,
  ).toBe("timed");
  expect(schedulePatch({ ...v, end: "" }).scheduled).toMatchObject({
    endAt: null,
  });
  expect(() => schedulePatch({ ...v, end: v.start })).toThrow();
  expect(
    scheduleDraft(
      {
        kind: "date_range",
        startDate: "2026-10-05",
        endDate: null,
        zone: "UTC",
      },
      null,
      "UTC",
    ).end,
  ).toBe("");
  expect(
    scheduleDraft(
      {
        kind: "timed",
        startAt: "2026-10-05T10:00:00Z",
        endAt: null,
        zone: "UTC",
      },
      null,
      "UTC",
    ).end,
  ).toBe("");
});

it("compares deadline boundaries across zones without treating inclusive end as overdue", () => {
  const s: Scheduled = {
    kind: "date_range",
    startDate: "2026-03-29",
    endDate: "2026-03-29",
    zone: "Europe/Berlin",
  };
  expect(
    scheduleExceedsDeadline(s, {
      kind: "date",
      date: "2026-03-29",
      zone: "Europe/Berlin",
    }),
  ).toBe(false);
  expect(
    scheduleExceedsDeadline(
      { ...s, endDate: null },
      { kind: "date", date: "2026-03-28", zone: "UTC" },
    ),
  ).toBe(true);
  expect(
    scheduleExceedsDeadline(
      {
        kind: "timed",
        startAt: "2026-10-05T10:00:00Z",
        endAt: null,
        zone: "UTC",
      },
      { kind: "instant", at: "2026-10-05T09:00:00Z", zone: "UTC" },
    ),
  ).toBe(true);
  expect(
    scheduleExceedsDeadline(
      { ...s, zone: "bad" },
      { kind: "date", date: "2026-03-29", zone: "UTC" },
    ),
  ).toBe(false);
});

it("timed exclusive midnight does not paint a fabricated extra day", () => {
  expect(
    scheduleDates({
      kind: "timed",
      startAt: "2026-10-05T21:00:00Z",
      endAt: "2026-10-06T21:00:00Z",
      zone: "Europe/Moscow",
    }),
  ).toEqual(["2026-10-06", "2026-10-06"]);
});
