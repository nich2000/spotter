import { it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { Planning, planningTasks, scheduleLabel } from "./Planning";
import { APIError, type Task, type Workspace, localDate } from "./api";
import { addDays } from "./calendar";
const today = localDate("Europe/Moscow");
const task = (id: string, patch: Partial<Task> = {}): Task =>
  ({
    id,
    title: id,
    projectId: "child",
    lifecycleState: "not_started",
    estimateMinutes: 60,
    scheduled: {
      kind: "date_range",
      startDate: today,
      endDate: addDays(today, 2),
      zone: "Europe/Moscow",
    },
    ...patch,
  }) as Task;
const w: Workspace = {
  tasks: [
    task("Range"),
    task("Start", {
      scheduled: {
        kind: "date_range",
        startDate: today,
        endDate: null,
        zone: "Europe/Moscow",
      },
      estimateMinutes: null,
    }),
    task("Deadline", {
      scheduled: null,
      deadline: { kind: "date", date: addDays(today, 5), zone: "UTC" },
      projectId: null,
    }),
    task("Future", {
      scheduled: {
        kind: "date_range",
        startDate: addDays(today, 50),
        endDate: addDays(today, 52),
        zone: "UTC",
      },
    }),
    task("Done", { lifecycleState: "done", relatedTaskIds: ["Range"] }),
    task("Hidden", { lifecycleState: "done" }),
    task("Pool", { scheduled: null }),
    task("Wait", {
      workMode: "waiting",
      deadline: { kind: "date", date: today, zone: "UTC" },
    }),
  ],
  projects: [
    { id: "p", name: "Parent", parentId: null },
    { id: "child", name: "Child", parentId: "p" },
  ],
  plans: [
    {
      id: "plan",
      date: today,
      status: "active",
      items: [{ taskId: "Range", position: 0, allocatedMinutes: 60 }],
      mainOccurrenceId: null,
      frogOccurrenceId: null,
    },
  ],
  settings: { zone: "Europe/Moscow" },
  days: {},
  migration: {},
};
beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  HTMLElement.prototype.setPointerCapture = vi.fn();
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    width: 1400,
  } as DOMRect);
});
function drag(name = "Range", delta = 100, resize = false) {
  const bar = screen.getByRole("button", {
    name: new RegExp("^" + name + ":"),
  });
  fireEvent.pointerDown(resize ? bar.querySelector("span")! : bar, {
    button: 0,
    clientX: 0,
    pointerId: 1,
  });
  fireEvent.pointerMove(bar, { clientX: delta, pointerId: 1 });
  return bar;
}
it("renders full hierarchy, linked completed rows, markers, full list and scale navigation", () => {
  const open = vi.fn();
  render(<Planning workspace={w} open={open} />);
  expect(screen.queryByText("Hidden")).not.toBeInTheDocument();
  expect(screen.getByText("Done")).toBeInTheDocument();
  expect(screen.getByText("Вне периода")).toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("Масштаб"), {
    target: { value: "90" },
  });
  expect(screen.queryByText("Вне периода")).not.toBeInTheDocument();
  fireEvent.click(screen.getByLabelText("Следующий период"));
  fireEvent.click(screen.getByLabelText("Предыдущий период"));
  fireEvent.click(screen.getByText("Сегодня"));
  fireEvent.click(screen.getByLabelText("Развернуть или свернуть Parent"));
  expect(screen.queryByText("Range")).not.toBeInTheDocument();
  fireEvent.click(screen.getByLabelText("Развернуть или свернуть Parent"));
  fireEvent.change(screen.getByLabelText("Фильтр проектов"), {
    target: { value: "p" },
  });
  expect(screen.getByText("Range")).toBeInTheDocument();
  expect(screen.queryByText("Deadline")).not.toBeInTheDocument();
  fireEvent.click(screen.getByText("Назначить окончание"));
  expect(open).toHaveBeenCalledWith(w.tasks[1]);
  fireEvent.click(screen.getByText("Назначить даты"));
  expect(open).toHaveBeenCalledWith(w.tasks[6]);
  fireEvent.change(screen.getByLabelText("Фильтр проектов"), {
    target: { value: "" },
  });
  fireEvent.click(screen.getByText("Назначить начало"));
  expect(open).toHaveBeenCalledWith(w.tasks[2]);
  fireEvent.doubleClick(screen.getByRole("button", { name: /^Range:/ }));
  expect(open).toHaveBeenCalledWith(w.tasks[0]);
  fireEvent.keyDown(screen.getByRole("button", { name: /^Wait:/ }), {
    key: "Enter",
  });
  expect(open).toHaveBeenCalledWith(w.tasks[7]);
  expect(planningTasks(w, "p")).toHaveLength(6);
});
it("commits a single frozen proposal on release, preserves original bounds and retries exactly", async () => {
  const commit = vi
    .fn()
    .mockRejectedValueOnce(new Error("Network"))
    .mockResolvedValue({ warnings: ["SCHEDULE_EXCEEDS_DEADLINE"] });
  const open = vi.fn();
  const { rerender } = render(
    <Planning workspace={w} revision={7} open={open} commit={commit} />,
  );
  const bar = drag();
  expect(commit).not.toHaveBeenCalled();
  rerender(<Planning workspace={w} revision={8} open={open} commit={commit} />);
  fireEvent.pointerUp(bar);
  await screen.findByText("Повторить тот же запрос");
  expect(commit.mock.calls[0][0]).toMatchObject({
    expectedRevision: 7,
    command: "task.save",
    payload: {
      taskId: "Range",
      patch: {
        scheduled: { startDate: addDays(today, 1), endDate: addDays(today, 3) },
      },
    },
  });
  fireEvent.doubleClick(bar);
  expect(open).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText("Повторить тот же запрос"));
  await screen.findByText("Сохранено. План выходит за дедлайн.");
  expect(commit.mock.calls[1][0]).toEqual(commit.mock.calls[0][0]);
});
it("cancels gestures and zero moves, rejects invalid ends, and resizes only end", async () => {
  const commit = vi.fn().mockResolvedValue({}),
    open = vi.fn();
  render(<Planning workspace={w} open={open} commit={commit} />);
  let bar = drag();
  fireEvent.keyDown(window, { key: "Escape" });
  fireEvent.pointerUp(bar);
  expect(commit).not.toHaveBeenCalled();
  bar = drag();
  fireEvent.pointerCancel(bar);
  fireEvent.pointerUp(bar);
  expect(commit).not.toHaveBeenCalled();
  bar = drag("Range", 0);
  fireEvent.pointerUp(bar);
  expect(commit).not.toHaveBeenCalled();
  bar = drag("Range", -500, true);
  fireEvent.pointerUp(bar);
  expect(commit).not.toHaveBeenCalled();
  bar = drag("Range", 100, true);
  fireEvent.pointerUp(bar);
  await screen.findByText("Сроки сохранены");
  expect(commit.mock.calls[0][0].payload.patch.scheduled).toMatchObject({
    startDate: today,
    endDate: addDays(today, 3),
  });
});
it("conflict reloads actual data and opens retained proposal only by explicit choice", async () => {
  const commit = vi
      .fn()
      .mockRejectedValue(new APIError("VERSION_CONFLICT", "Conflict", 409)),
    refresh = vi.fn().mockResolvedValue(undefined),
    open = vi.fn();
  render(
    <Planning workspace={w} open={open} commit={commit} refresh={refresh} />,
  );
  fireEvent.pointerUp(drag());
  await screen.findByText("Открыть черновик в карточке");
  expect(refresh).toHaveBeenCalledOnce();
  fireEvent.click(screen.getByText("Открыть черновик в карточке"));
  expect(open).toHaveBeenCalledWith(
    w.tasks[0],
    expect.objectContaining({ startDate: addDays(today, 1) }),
  );
  fireEvent.pointerUp(drag());
  await screen.findByText("Отменить черновик");
  fireEvent.click(screen.getByText("Отменить черновик"));
  expect(screen.getByRole("status")).toHaveTextContent("Черновик отменён");
});
it("deep links select a task and navigate to its interval or deadline; empty pool and date labels", () => {
  const { rerender } = render(
    <Planning workspace={w} open={vi.fn()} query="?task=Future&project=p" />,
  );
  expect(screen.getByRole("button", { name: /^Future:/ })).toBeInTheDocument();
  rerender(
    <Planning
      workspace={{ ...w, tasks: [w.tasks[2]] }}
      open={vi.fn()}
      query="?task=Deadline"
    />,
  );
  expect(
    screen.queryByRole("heading", { name: /Без дат/ }),
  ).not.toBeInTheDocument();
  rerender(
    <Planning workspace={{ ...w, tasks: [] }} open={vi.fn()} query="" />,
  );
  expect(screen.getByText("Нет задач")).toBeInTheDocument();
  expect(
    scheduleLabel({
      kind: "timed",
      startAt: "2026-10-05T10:00:00Z",
      endAt: null,
      zone: "UTC",
    }),
  ).toContain("10:00:00 · без окончания");
  expect(
    scheduleLabel({
      kind: "timed",
      startAt: "2026-10-05T10:00:00Z",
      endAt: "2026-10-05T11:00:00Z",
      zone: "UTC",
    }),
  ).toContain("11:00:00");
});
it("moves a clipped interval using its full stored bounds", async () => {
  const clipped = task("Clipped", {
    scheduled: {
      kind: "date_range",
      startDate: addDays(today, -5),
      endDate: addDays(today, 3),
      zone: "UTC",
    },
  });
  const commit = vi.fn().mockResolvedValue({});
  render(
    <Planning
      workspace={{ ...w, tasks: [clipped] }}
      open={vi.fn()}
      commit={commit}
    />,
  );
  fireEvent.pointerUp(drag("Clipped", 100));
  await screen.findByText("Сроки сохранены");
  expect(commit.mock.calls[0][0].payload.patch.scheduled).toMatchObject({
    startDate: addDays(today, -4),
    endDate: addDays(today, 4),
  });
});
