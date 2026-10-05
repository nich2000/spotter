import { it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import {
  Inbox,
  ProjectTree,
  Planning,
  Workday,
  Statistics,
} from "./MockupViews";
import { Task, Workspace } from "./api";
const t = {
  id: "t",
  title: "One",
  lifecycleState: "not_started",
  importance: "unknown",
  urgency: "unknown",
  reviewRequired: true,
  estimateMinutes: 30,
  projectId: "child",
  subtasks: [],
  context: "away",
} as unknown as Task;
const workspace: Workspace = {
  tasks: [
    t,
    { ...t, id: "done", title: "Done", lifecycleState: "done" },
    {
      ...t,
      id: "waiting",
      title: "Waiting",
      lifecycleState: "in_progress",
      workMode: "waiting",
    },
  ],
  projects: [
    { id: "p", name: "Parent", parentId: null },
    { id: "child", name: "Child", parentId: "p" },
  ],
  plans: [],
  settings: { reservePercent: 25 },
  days: {},
  migration: {},
  candidates: [
    {
      id: "c",
      proposedTitle: "Proposal",
      sourceId: "mail",
      candidateVersion: 1,
      sourceSnapshotId: "s",
      state: "pending",
    },
    {
      id: "d",
      proposedTitle: "Skipped",
      sourceId: "mail",
      candidateVersion: 1,
      sourceSnapshotId: "s",
      state: "dismissed",
    },
  ],
};
const card = (t: Task) => <span key={t.id}>{t.title}</span>;
it("displays matrix simultaneously, captures tasks and moves them without changing other fields", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        items: [
          { name: "mail", snapshot: { ok: true } },
          { name: "notes", snapshot: { ok: false } },
        ],
      }),
    }),
  );
  const command = vi.fn().mockResolvedValue(undefined);
  render(<Inbox workspace={workspace} command={command} card={card} />);
  await screen.findByText("Почта · получен снимок");
  expect(screen.getAllByRole("region")).toHaveLength(4);
  fireEvent.change(screen.getByLabelText("Быстро во Входящие"), {
    target: { value: "New" },
  });
  fireEvent.click(screen.getByText("Добавить во Входящие"));
  await waitFor(() =>
    expect(command).toHaveBeenCalledWith("task.create", { title: "New" }),
  );
  const transfer = { getData: () => "t" };
  fireEvent.dragOver(screen.getByRole("region", { name: "Важно и срочно" }));
  fireEvent.drop(screen.getByRole("region", { name: "Важно и срочно" }), {
    dataTransfer: transfer,
  });
  await waitFor(() =>
    expect(command).toHaveBeenCalledWith("task.triage", {
      taskId: "t",
      triage: { action: "complete", importance: "yes", urgency: "yes" },
    }),
  );
  fireEvent.drop(screen.getByRole("region", { name: "Не важно, не срочно" }), {
    dataTransfer: transfer,
  });
  fireEvent.drop(screen.getByRole("region", { name: "Не важно, не срочно" }), {
    dataTransfer: { getData: () => "invalid" },
  });
  fireEvent.click(screen.getByText("Принять"));
  fireEvent.click(screen.getByText("Пропустить"));
  fireEvent.click(screen.getByText("Вернуть в предложения системы"));
  await waitFor(() =>
    expect(command.mock.calls.map((c) => c[0])).toEqual(
      expect.arrayContaining([
        "source.accept",
        "source.dismiss",
        "source.restore",
      ]),
    ),
  );
  fireEvent.click(screen.getByText("Обновить статусы"));
  fireEvent.change(screen.getByLabelText("Поиск задач"), {
    target: { value: "missing" },
  });
  expect(screen.queryByText("One")).not.toBeInTheDocument();
  command.mockRejectedValue(new Error("Rejected"));
  fireEvent.click(screen.getByText("Принять"));
  expect(await screen.findByRole("alert")).toHaveTextContent("Rejected");
  fireEvent.change(screen.getByLabelText("Быстро во Входящие"), {
    target: { value: "Keep draft" },
  });
  fireEvent.click(screen.getByText("Добавить во Входящие"));
  await waitFor(() =>
    expect(screen.getByLabelText("Быстро во Входящие")).toHaveValue("Keep draft"),
  );
});
it("aggregates descendants and collapses project branches", async () => {
  const command = vi.fn().mockResolvedValue(undefined);
  render(<ProjectTree workspace={workspace} command={command} card={card} />);
  expect(screen.getByText("One")).toBeInTheDocument();
  expect(screen.getAllByText("2 открытых · 1 завершено")).toHaveLength(2);
  fireEvent.click(screen.getByLabelText("Развернуть или свернуть Parent"));
  expect(screen.queryByText("One")).not.toBeInTheDocument();
  fireEvent.click(screen.getByLabelText("Развернуть или свернуть Parent"));
  fireEvent.click(screen.getByRole("button", { name: "Child" }));
  fireEvent.change(screen.getByLabelText("Название проекта"), {
    target: { value: "Nested" },
  });
  fireEvent.click(screen.getByText("Создать"));
  await waitFor(() =>
    expect(command).toHaveBeenCalledWith("project.create", {
      name: "Nested",
      parentId: "child",
    }),
  );
  fireEvent.click(screen.getByText("Без проекта"));
  fireEvent.change(screen.getByLabelText("Родительский проект"), {
    target: { value: "p" },
  });
  command.mockRejectedValue(new Error("Conflict"));
  fireEvent.change(screen.getByLabelText("Название проекта"), {
    target: { value: "Preserved" },
  });
  fireEvent.click(screen.getByText("Создать"));
  expect(await screen.findByRole("alert")).toHaveTextContent("Conflict");
});
it("workday groups waiting and completed tasks and opens the main result", () => {
  const open = vi.fn();
  render(
    <Workday
      workspace={workspace}
      plan={{
        id: "p",
        date: "2026-10-05",
        status: "active",
        items: workspace.tasks.map((t, i) => ({
          taskId: t.id,
          position: i,
          allocatedMinutes: 30,
        })),
        mainOccurrenceId: "t",
        frogOccurrenceId: null,
      }}
      card={card}
      open={open}
    />,
  );
  fireEvent.click(screen.getByText("Открыть карточку задачи"));
  expect(open).toHaveBeenCalledWith(t);
  expect(screen.getByText("Waiting")).toBeInTheDocument();
  expect(screen.getByText("Начать фокус")).toBeDisabled();
});
it("statistics renders report sections without fictional metrics", () => {
  render(<Statistics />);
  expect(screen.getByText("Движение по проектам")).toBeInTheDocument();
  expect(screen.getAllByText("—")).toHaveLength(4);
});

it("separates unreviewed tasks and system proposals into independent collapsible blocks", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({ ok: true, json: async () => ({ items: [] }) }),
  );
  const { container } = render(
    <Inbox workspace={workspace} command={vi.fn()} card={card} />,
  );
  await screen.findByText("Календарь · нет снимка");
  const blocks =
    container.querySelectorAll<HTMLDetailsElement>(".intake-block");
  expect(blocks).toHaveLength(2);
  expect(blocks[0]).toHaveTextContent("Неразобранное 2");
  expect(blocks[0]).toHaveTextContent("One");
  expect(blocks[0]).not.toHaveTextContent("Proposal");
  expect(blocks[1]).toHaveTextContent("Предложения системы 1");
  expect(blocks[1]).toHaveTextContent("Proposal");
  expect(blocks[1]).not.toHaveTextContent("Skipped");
  expect(blocks[0].open).toBe(true);
  expect(blocks[1].open).toBe(true);
  fireEvent.click(blocks[0].querySelector("summary")!);
  expect(blocks[0].open).toBe(false);
  expect(blocks[1].open).toBe(true);
  fireEvent.click(blocks[1].querySelector("summary")!);
  expect(blocks[1].open).toBe(false);
  fireEvent.click(blocks[0].querySelector("summary")!);
  expect(blocks[0].open).toBe(true);
  expect(blocks[1].open).toBe(false);
});
