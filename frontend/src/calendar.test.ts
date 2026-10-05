import { it, expect } from "vitest";
import {
  addDays,
  dayNumber,
  displayDate,
  displayInstant,
  dateInZone,
  visibleInterval,
} from "./calendar";
it("uses the requested date format and explicit zone", () => {
  expect(displayDate("2026-10-05")).toBe("2026.10.05");
  expect(displayInstant("2026-10-05T09:02:03Z", "Europe/Moscow")).toBe(
    "2026.10.05 12:02:03",
  );
  expect(dateInZone("2026-10-05T23:30:00Z", "Europe/Moscow")).toBe(
    "2026-10-06",
  );
});
it("preserves calendar days across month, leap year and DST boundaries", () => {
  expect(addDays("2026-12-31", 1)).toBe("2027-01-01");
  expect(addDays("2024-02-28", 1)).toBe("2024-02-29");
  expect(addDays("2026-03-28", 2)).toBe("2026-03-30");
  expect(addDays("2026-10-26", -2)).toBe("2026-10-24");
  expect(() => dayNumber("2026-02-29")).toThrow();
  expect(() => dayNumber("05.10.2026")).toThrow();
  expect(() => addDays("2026-10-05", 0.5)).toThrow();
});
it("clips intersecting intervals without changing original boundaries and supports zoom", () => {
  expect(visibleInterval("2026-09-28", "2026-10-07", "2026-10-05", 14)).toEqual(
    { left: 0, width: (3 / 14) * 100 },
  );
  expect(visibleInterval("2026-10-05", "2026-10-05", "2026-10-05", 14)).toEqual(
    { left: 0, width: (1 / 14) * 100 },
  );
  expect(
    visibleInterval("2026-11-05", "2026-11-07", "2026-10-05", 14),
  ).toBeNull();
  expect(
    visibleInterval("2026-11-05", "2026-11-07", "2026-10-05", 90),
  ).not.toBeNull();
  expect(() =>
    visibleInterval("2026-10-07", "2026-10-05", "2026-10-05", 14),
  ).toThrow();
  expect(() =>
    visibleInterval("2026-10-05", "2026-10-07", "2026-10-05", 0),
  ).toThrow();
});
