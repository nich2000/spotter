import { it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TaskEditor } from "./TaskEditor";
import { Task } from "./api";
const task = {
  id: "t",
  title: "Original",
  description: "Body",
  resume: "Next",
  projectId: "p",
  estimateMinutes: 20,
  context: "anywhere",
  complexity: "easy",
  multiDay: false,
  subtasks: [{ id: "s", title: "Step", done: false, position: 0 }],
  comments: [{ id: "c", text: "Earlier", createdAt: "2026-10-05T10:00:00Z" }],
} as Task;
it("edits the full card in one atomic command", async () => {
  const save = vi.fn().mockResolvedValue(undefined),
    close = vi.fn();
  render(
    <TaskEditor
      task={task}
      projects={[{ id: "p", name: "Work", parentId: null }]}
      save={save}
      close={close}
    />,
  );
  fireEvent.change(screen.getByLabelText("Название"), {
    target: { value: "Changed" },
  });
  fireEvent.change(screen.getByLabelText("Описание"), {
    target: { value: "New description" },
  });
  fireEvent.change(screen.getByLabelText("Заметка для возвращения"), {
    target: { value: "Resume here" },
  });
  fireEvent.change(screen.getByLabelText("Оценка, мин"), {
    target: { value: "35" },
  });
  fireEvent.change(screen.getByLabelText("Контекст"), {
    target: { value: "computer" },
  });
  fireEvent.change(screen.getByLabelText("Сложность"), {
    target: { value: "hard" },
  });
  fireEvent.click(screen.getByLabelText("Многодневная"));
  fireEvent.click(screen.getByLabelText("Готово: Step"));
  fireEvent.change(screen.getByLabelText("Подпункт 1"), {
    target: { value: "Verified" },
  });
  fireEvent.change(screen.getByLabelText("Новый комментарий"), {
    target: { value: "A note" },
  });
  fireEvent.click(screen.getByText("Сохранить"));
  await waitFor(() => expect(close).toHaveBeenCalled());
  expect(save).toHaveBeenCalledWith(
    "task.save",
    expect.objectContaining({
      taskId: "t",
      patch: expect.objectContaining({
        title: "Changed",
        estimateMinutes: 35,
        context: "computer",
        complexity: "hard",
        multiDay: true,
      }),
      commentsToAdd: [{ clientRef: "comment", text: "A note" }],
      subtasks: [{ id: "s", title: "Verified", done: true, position: 0 }],
    }),
  );
});
it("retains draft after conflict and cancellation never writes", async () => {
  const save = vi
      .fn()
      .mockRejectedValue(new Error("Conflict — draft retained")),
    close = vi.fn();
  render(<TaskEditor task={null} projects={[]} save={save} close={close} />);
  fireEvent.change(screen.getByLabelText("Название"), {
    target: { value: "Draft" },
  });
  fireEvent.click(screen.getByText("+ Подпункт"));
  fireEvent.change(screen.getByLabelText("Подпункт 1"), {
    target: { value: "One" },
  });
  fireEvent.click(screen.getByLabelText("Удалить подпункт 1"));
  fireEvent.click(screen.getByText("Сохранить"));
  expect(await screen.findByRole("alert")).toHaveTextContent("Conflict");
  expect(screen.getByLabelText("Название")).toHaveValue("Draft");
  expect(close).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText("Отмена"));
  expect(close).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText("Отбросить изменения"));
  expect(close).toHaveBeenCalledOnce();
  expect(save).toHaveBeenCalledOnce();
});
it("closes from header", () => {
  const close = vi.fn();
  render(<TaskEditor task={null} projects={[]} save={vi.fn()} close={close} />);
  fireEvent.click(screen.getByLabelText("Закрыть карточку"));
  expect(close).toHaveBeenCalled();
});
it("requires explicit subtask completion decision and keeps one atomic save", async () => {
  const save = vi.fn().mockResolvedValue(undefined);
  render(<TaskEditor task={task} projects={[]} save={save} close={vi.fn()} />);
  fireEvent.change(screen.getByLabelText("Состояние"), {
    target: { value: "done" },
  });
  fireEvent.change(screen.getByLabelText("Как завершить задачу?"), {
    target: { value: "complete_all" },
  });
  fireEvent.change(screen.getByLabelText("Важность"), {
    target: { value: "yes" },
  });
  fireEvent.change(screen.getByLabelText("Срочность"), {
    target: { value: "no" },
  });
  fireEvent.change(screen.getByLabelText("Осталось, мин"), {
    target: { value: "0" },
  });
  fireEvent.click(screen.getByText("Сохранить"));
  await waitFor(() =>
    expect(save).toHaveBeenCalledWith(
      "task.save",
      expect.objectContaining({
        transition: {
          to: "done",
          completionActor: "self",
          unfinishedSubtasksDecision: "complete_all",
        },
        triage: { action: "complete", importance: "yes", urgency: "no" },
        patch: expect.objectContaining({ remainingMinutes: 0 }),
      }),
    ),
  );
});
it("saves waiting reason and check date; Escape protects the draft", async () => {
  const save = vi.fn().mockResolvedValue(undefined),
    close = vi.fn();
  render(<TaskEditor task={task} projects={[]} save={save} close={close} />);
  fireEvent.change(screen.getByLabelText("Состояние"), {
    target: { value: "waiting" },
  });
  fireEvent.change(screen.getByLabelText("Причина / следующий шаг"), {
    target: { value: "Reply" },
  });
  fireEvent.change(screen.getByLabelText("Когда проверить"), {
    target: { value: "2026-10-06T12:00" },
  });
  fireEvent.keyDown(document, { key: "Escape" });
  expect(close).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText("Продолжить редактирование"));
  fireEvent.click(screen.getByText("Сохранить"));
  await waitFor(() =>
    expect(save).toHaveBeenCalledWith(
      "task.save",
      expect.objectContaining({
        transition: expect.objectContaining({
          to: "in_progress",
          workMode: "waiting",
          waitingOn: "Reply",
        }),
      }),
    ),
  );
});
it("editing title preserves review-required priority until explicitly triaged", async () => {
  const save = vi.fn().mockResolvedValue(undefined);
  render(
    <TaskEditor
      task={{ ...task, importance: "yes", urgency: "no", reviewRequired: true }}
      projects={[]}
      save={save}
      close={vi.fn()}
    />,
  );
  fireEvent.change(screen.getByLabelText("Название"), {
    target: { value: "Renamed" },
  });
  fireEvent.click(screen.getByText("Сохранить"));
  await waitFor(() => expect(save).toHaveBeenCalled());
  expect(save.mock.calls[0][1].triage).toBeUndefined();
  expect(save.mock.calls[0][1].patch).toMatchObject({
    importance: "yes",
    urgency: "no",
  });
});

it("edits calendar dates and explicit task links together, then clears just the schedule", async () => {
  const save = vi.fn().mockResolvedValue(undefined);
  const linked = { ...task, id: "other", title: "Linked" };
  render(
    <TaskEditor
      task={task}
      tasks={[task, linked]}
      projects={[]}
      save={save}
      close={vi.fn()}
      focusPlanning
    />,
  );
  fireEvent.change(screen.getByLabelText("Тип интервала"), {
    target: { value: "date_range" },
  });
  fireEvent.change(screen.getByLabelText("Начало"), {
    target: { value: "2026-10-05" },
  });
  fireEvent.change(screen.getByLabelText("Окончание, включительно"), {
    target: { value: "2026-10-07" },
  });
  fireEvent.change(screen.getByLabelText("Часовой пояс интервала"), {
    target: { value: "Europe/Berlin" },
  });
  fireEvent.change(screen.getByLabelText("Тип дедлайна"), {
    target: { value: "date" },
  });
  fireEvent.change(screen.getByLabelText("Дедлайн"), {
    target: { value: "2026-10-06" },
  });
  fireEvent.change(screen.getByLabelText("Часовой пояс дедлайна"), {
    target: { value: "UTC" },
  });
  fireEvent.change(screen.getByLabelText("Добавить связь"), {
    target: { value: "other" },
  });
  fireEvent.click(screen.getByText("Убрать связь"));
  fireEvent.change(screen.getByLabelText("Добавить связь"), {
    target: { value: "other" },
  });
  fireEvent.click(screen.getByText("Сохранить"));
  await waitFor(() => expect(save).toHaveBeenCalledOnce());
  expect(save.mock.calls[0][1].patch).toMatchObject({
    relatedTaskIds: ["other"],
    scheduled: {
      kind: "date_range",
      startDate: "2026-10-05",
      endDate: "2026-10-07",
      zone: "Europe/Berlin",
    },
    deadline: { kind: "date", date: "2026-10-06", zone: "UTC" },
  });
  fireEvent.click(screen.getByText("Убрать из расписания"));
  fireEvent.click(screen.getByText("Сохранить"));
  await waitFor(() => expect(save).toHaveBeenCalledTimes(2));
  expect(save.mock.calls[1][1].patch.scheduled).toBeNull();
  expect(save.mock.calls[1][1].patch.deadline).not.toBeNull();
});
it("validates exact local time and retains invalid DST drafts", async () => {
  const save = vi.fn().mockResolvedValue(undefined);
  render(<TaskEditor task={null} projects={[]} save={save} close={vi.fn()} />);
  fireEvent.change(screen.getByLabelText("Название"), {
    target: { value: "DST" },
  });
  fireEvent.change(screen.getByLabelText("Тип интервала"), {
    target: { value: "timed" },
  });
  fireEvent.change(screen.getByLabelText("Часовой пояс интервала"), {
    target: { value: "Europe/Berlin" },
  });
  fireEvent.change(screen.getByLabelText("Начало"), {
    target: { value: "2026-10-25T02:30:00" },
  });
  fireEvent.click(screen.getByText("Сохранить"));
  expect(await screen.findByRole("alert")).toHaveTextContent("неоднозначно");
  expect(save).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText("Неоднозначное время DST"), {
    target: { value: "later" },
  });
  fireEvent.change(screen.getByLabelText("Окончание"), {
    target: { value: "2026-10-25T03:30:00" },
  });
  fireEvent.click(screen.getByText("Сохранить"));
  await waitFor(() => expect(save).toHaveBeenCalledOnce());
  expect(save.mock.calls[0][1].patch.scheduled.startAt).toBe(
    "2026-10-25T01:30:00.000Z",
  );
});

it("requires explicit revision refresh after conflict and preserves entered dates", async () => {
  const { APIError } = await import("./api");
  const save = vi
    .fn()
    .mockRejectedValueOnce(new APIError("VERSION_CONFLICT", "Conflict", 409))
    .mockResolvedValue(undefined);
  const resolveConflict = vi
    .fn()
    .mockResolvedValue({
      ...task,
      scheduled: {
        kind: "date_range",
        startDate: "2026-10-09",
        endDate: null,
        zone: "UTC",
      },
    });
  render(
    <TaskEditor
      task={task}
      projects={[]}
      save={save}
      resolveConflict={resolveConflict}
      close={vi.fn()}
    />,
  );
  fireEvent.change(screen.getByLabelText("Тип интервала"), {
    target: { value: "date_range" },
  });
  fireEvent.change(screen.getByLabelText("Начало"), {
    target: { value: "2026-10-05" },
  });
  fireEvent.click(screen.getByText("Сохранить"));
  await screen.findByText("Загрузить актуальную версию, оставить черновик");
  expect(screen.getByText("Сохранить")).toBeDisabled();
  fireEvent.click(
    screen.getByText("Загрузить актуальную версию, оставить черновик"),
  );
  await waitFor(() => expect(screen.getByText("Сохранить")).toBeEnabled());
  expect(screen.getByLabelText("Начало")).toHaveValue("2026-10-05");
  expect(screen.getByRole("alert")).toHaveTextContent("2026.10.09");
  fireEvent.click(screen.getByText("Сохранить"));
  await waitFor(() => expect(save).toHaveBeenCalledTimes(2));
});
it("locks the draft after an unknown transport outcome while allowing retry", async () => {
  const save = vi
    .fn()
    .mockRejectedValueOnce(new TypeError("Failed to fetch"))
    .mockResolvedValue(undefined);
  render(<TaskEditor task={task} projects={[]} save={save} close={vi.fn()} />);
  fireEvent.click(screen.getByText("Сохранить"));
  await screen.findByRole("alert");
  expect(screen.getByLabelText("Название")).toBeDisabled();
  expect(screen.getByText("Сохранить")).toBeEnabled();
  fireEvent.click(screen.getByText("Сохранить"));
  await waitFor(() => expect(save).toHaveBeenCalledTimes(2));
});
