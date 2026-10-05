import { it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { CalendarView } from "./CalendarView";
import {
  monthDays,
  shiftMonth,
  weekRibbons,
  layoutEntries,
  dayEntries,
  onDay,
  dueOn,
} from "./calendarLayout";
import { localDate, APIError, type Workspace, type Task } from "./api";
import { WorkScheduleFields } from "./WorkScheduleFields";
const date = localDate("UTC");
const t = {
  id: "t",
  title: "Task",
  lifecycleState: "not_started",
  projectId: null,
  scheduled: {
    kind: "date_range",
    startDate: date,
    endDate: date,
    zone: "UTC",
  },
  deadline: { kind: "date", date, zone: "UTC" },
} as Task;
const w: Workspace = {
  tasks: [t],
  projects: [],
  plans: [],
  days: {},
  settings: { zone: "UTC", workStart: "09:00", workEnd: "18:00" },
  migration: {},
};
const block = {
  id: "b",
  taskId: "t",
  title: "Task",
  kind: "work" as const,
  startAt: date + "T10:00:00Z",
  endAt: date + "T10:25:00Z",
};
const setup = (workspace = w, commit = vi.fn().mockResolvedValue({})) => {
  const open = vi.fn(),
    refresh = vi.fn().mockResolvedValue({});
  render(
    <CalendarView
      workspace={workspace}
      revision={2}
      open={open}
      commit={commit}
      refresh={refresh}
    />,
  );
  return { open, commit, refresh };
};
it("lays out leap months, year changes, weekly ribbons and overlapping midnight events", () => {
  expect(monthDays("2024-02-10")).toContain("2024-02-29");
  expect(monthDays("2026-02-01")[0]).toBe("2026-01-26");
  expect(shiftMonth("2026-12-05", 1)).toBe("2027-01-01");
  expect(onDay(t, date)).toBe(true);
  expect(onDay({ ...t, scheduled: null }, date)).toBe(false);
  expect(dueOn(t, date)).toBe(true);
  const week = monthDays(date)
    .filter((d) => d >= date)
    .slice(0, 7);
  expect(
    weekRibbons([t, { ...t, id: "second" }], week).map((r) => r.lane),
  ).toEqual([0, 1]);
  const e = {
    ...block,
    startAt: date + "T00:00:00Z",
    endAt: date + "T02:00:00Z",
  };
  const rows = layoutEntries(
    [
      e,
      { ...e, id: "other" },
      {
        ...e,
        id: "later",
        startAt: date + "T03:00:00Z",
        endAt: date + "T04:00:00Z",
      },
    ],
    date,
    "UTC",
  );
  expect(rows.map((r) => r.lanes)).toEqual([2, 2, 1]);
  expect(
    dayEntries([{ ...e, endAt: date + "T00:00:00Z" }], date, "UTC"),
  ).toHaveLength(1);
});
it("navigates calendar, opens a task and saves only after preview confirmation", async () => {
  const fetch = vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ blocks: [block], workspaceRevision: 2 }),
  });
  vi.stubGlobal("fetch", fetch);
  const { open, commit } = setup();
  fireEvent.click(screen.getByLabelText("Следующий месяц"));
  fireEvent.click(screen.getByLabelText("Предыдущий месяц"));
  fireEvent.click(screen.getByText("Сегодня"));
  fireEvent.click(screen.getByText("Открыть"));
  expect(open).toHaveBeenCalledWith(t);
  fireEvent.change(screen.getByLabelText("Задача"), { target: { value: "t" } });
  fireEvent.change(screen.getByLabelText("Начало"), {
    target: { value: "11:00" },
  });
  fireEvent.change(screen.getByLabelText("Работа, минут"), {
    target: { value: "50" },
  });
  fireEvent.click(screen.getByLabelText("Закрепить блоки"));
  fireEvent.click(screen.getByText("Предложить размещение"));
  await screen.findByText("Подтвердить");
  expect(commit).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText("Отмена"));
  expect(screen.queryByText("Подтвердить")).not.toBeInTheDocument();
  fireEvent.click(screen.getByText("Подобрать свободное время"));
  await screen.findByText("Подтвердить");
  fireEvent.click(screen.getByText("Подтвердить"));
  await waitFor(() => expect(commit).toHaveBeenCalled());
  expect(commit.mock.calls[0][0]).toMatchObject({
    command: "workblocks.create",
    expectedRevision: 2,
    payload: { taskId: "t", minutes: 50, pinned: true, automatic: true },
  });
});
it("retries the same envelope on unknown outcome and preserves external read-only events", async () => {
  const commit = vi
    .fn()
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValue({});
  const external = {
    ...block,
    id: "ext",
    taskId: undefined,
    title: "Meeting",
    kind: "external" as const,
    source: "Mac",
  };
  setup(
    {
      ...w,
      workBlocks: [block],
      calendarSources: [
        {
          id: "c",
          ok: true,
          events: [external],
          observedAt: date + "T00:00:00Z",
        },
      ],
    },
    commit,
  );
  expect(screen.getByText(/Mac · Только просмотр/)).toBeInTheDocument();
  fireEvent.click(screen.getByText("Убрать блок"));
  await screen.findByText("Повторить сохранение");
  fireEvent.click(screen.getByText("Повторить сохранение"));
  await waitFor(() => expect(commit).toHaveBeenCalledTimes(2));
  expect(commit.mock.calls[0][0]).toEqual(commit.mock.calls[1][0]);
  fireEvent.click(screen.getByText("Закрепить"));
  await waitFor(() => expect(commit).toHaveBeenCalledTimes(3));
});
it("reports proposal and revision errors without false success", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: false,
      status: 422,
      json: async () => ({
        error: { code: "INVALID", message: "Время занято" },
      }),
    }),
  );
  setup();
  fireEvent.change(screen.getByLabelText("Задача"), { target: { value: "t" } });
  fireEvent.click(screen.getByText("Предложить размещение"));
  await screen.findByText("Время занято");
  expect(screen.queryByText("Подтвердить")).not.toBeInTheDocument();
});

it("keeps a valid work block inside its task interval without warning", () => {
  setup({ ...w, workBlocks: [block] });
  expect(screen.queryByText(/Есть блоки за пределами/)).not.toBeInTheDocument();
});

it("retains the exact command when the server returns an uncertain 503", async () => {
  const commit = vi.fn().mockRejectedValueOnce(new APIError("SERVICE_UNAVAILABLE", "retry", 503)).mockResolvedValue({});
  setup({ ...w, workBlocks: [block] }, commit);
  fireEvent.click(screen.getByText("Убрать блок"));
  await screen.findByText("Повторить сохранение");
  fireEvent.click(screen.getByText("Повторить сохранение"));
  await waitFor(() => expect(commit).toHaveBeenCalledTimes(2));
  expect(commit.mock.calls[0][0]).toEqual(commit.mock.calls[1][0]);
});
it("edits work week, exceptions and pomodoro without saving implicitly", () => {
  const change = vi.fn();
  const { rerender } = render(
    <WorkScheduleFields settings={w.settings} onChange={change} />,
  );
  fireEvent.change(screen.getByLabelText("Рабочий интервал"), {
    target: { value: "30" },
  });
  expect(change).toHaveBeenLastCalledWith(
    expect.objectContaining({ pomodoroMinutes: 30 }),
  );
  fireEvent.click(screen.getByLabelText("Сб"));
  expect(change).toHaveBeenLastCalledWith(
    expect.objectContaining({
      workWeek: { 6: { start: "09:00", end: "18:00" } },
    }),
  );
  fireEvent.change(screen.getByLabelText("Добавить дату"), {
    target: { value: date },
  });
  rerender(
    <WorkScheduleFields
      settings={{
        ...w.settings,
        workExceptions: { [date]: { start: "10:00", end: "16:00" } },
      }}
      onChange={change}
    />,
  );
  fireEvent.change(screen.getByLabelText(date + " начало"), {
    target: { value: "11:00" },
  });
  fireEvent.click(screen.getByText("Убрать корректировку"));
  expect(change).toHaveBeenLastCalledWith(
    expect.objectContaining({ workExceptions: {} }),
  );
});
it("previews selected tasks across days and handles revision conflicts", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue({
        ok: true,
        json: async () => ({ blocks: [block], workspaceRevision: 2 }),
      }),
  );
  const commit = vi
    .fn()
    .mockRejectedValue(
      new APIError("VERSION_CONFLICT", "Данные изменились", 409),
    );
  const { refresh } = setup(w, commit);
  fireEvent.click(screen.getByText("Распределить выбранные задачи по дням"));
  fireEvent.click(screen.getByLabelText("Без проекта / Task"));
  fireEvent.change(screen.getByLabelText("До даты включительно"), {
    target: { value: date },
  });
  fireEvent.click(screen.getByText("Предложить распределение"));
  await screen.findByText("Подтвердить");
  fireEvent.click(screen.getByText("Подтвердить"));
  await screen.findByText("Данные изменились");
  expect(refresh).toHaveBeenCalled();
  expect(screen.queryByText("Подтвердить")).not.toBeInTheDocument();
});
