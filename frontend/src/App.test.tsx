import { beforeEach, it, expect, vi } from "vitest";
import {
  render,
  screen,
  fireEvent,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { App } from "./App";
import { Settings, Wellbeing } from "./Settings";
import { Task, Workspace, localDate } from "./api";
let authenticated = true,
  setup = false;
let calls: any[] = [];
let fail = false;
const task: Task = {
  id: "t",
  definitionId: "d",
  title: "Important task",
  description: "",
  resume: "Continue here",
  projectId: "p",
  context: "anywhere",
  importance: "unknown",
  urgency: "unknown",
  reviewRequired: true,
  estimateMinutes: null,
  remainingMinutes: null,
  complexity: null,
  multiDay: false,
  lifecycleState: "not_started",
  workMode: null,
  completionEventId: null,
  subtasks: [],
  comments: [],
};
let workspace: Workspace;
beforeEach(() => {
  authenticated = true;
  setup = false;
  calls = [];
  fail = false;
  workspace = {
    tasks: [
      { ...task },
      {
        ...task,
        id: "done",
        title: "Finished",
        lifecycleState: "done",
        completionEventId: "event",
        estimateMinutes: 20,
      },
    ],
    projects: [{ id: "p", name: "Work", parentId: null }],
    plans: [],
    settings: {
      zone: "Europe/Moscow",
      workStart: "09:30",
      workEnd: "18:00",
      reservePercent: 25,
      focusBlockMinutes: 90,
      scheduleBreakMinutes: 15,
      plannerSleepTargetHours: 8,
    },
    days: {},
    migration: {},
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init: any) => {
      calls.push([url, init]);
      const path = url.replace("/api/v2", "");
      if (fail && init.method === "POST")
        return {
          ok: false,
          status: 409,
          json: async () => ({
            error: { code: "VERSION_CONFLICT", message: "Conflict" },
          }),
        };
      let data: any = {};
      if (path === "/auth/session")
        data = { authenticated, setupRequired: setup };
      else if (path === "/workspace")
        data = { workspaceRevision: 3, data: workspace };
      else if (path === "/devices")
        data = {
          items: [{ id: "mac", name: "MacBook", active: true, lastSeen: null }],
        };
      else if (path === "/consents")
        data = {
          items: [
            { category: "diary", enabled: false },
            { category: "health", enabled: false },
          ],
        };
      else if (path === "/sources")
        data = {
          items: [
            {
              deviceId: "mac",
              name: "mail",
              snapshot: { ok: true },
              receivedAt: "2026-10-05T10:00:00Z",
            },
          ],
        };
      else if (path === "/device-enrollments") data = { code: "pair-code" };
      return { ok: true, json: async () => data };
    }),
  );
});
function app(path = "/day") {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  );
}
it("runs inbox card, triage, completion and project flow", async () => {
  app("/inbox");
  await screen.findByText("Important task");
  fireEvent.change(screen.getByLabelText("Поиск задач"), {
    target: { value: "absent" },
  });
  expect(screen.queryByText("Important task")).not.toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("Поиск задач"), {
    target: { value: "" },
  });
  fireEvent.click(screen.getByText("✓ Завершить"));
  await waitFor(() =>
    expect(calls.some((c) => c[1].body?.includes("task.transition"))).toBe(
      true,
    ),
  );
  fireEvent.click(screen.getAllByText("Важно · срочно").at(-1)!);
  await waitFor(() =>
    expect(calls.some((c) => c[1].body?.includes("task.triage"))).toBe(true),
  );
  fireEvent.click(screen.getByText("Important task"));
  await screen.findByRole("dialog");
  fireEvent.click(screen.getByText("Отмена"));
  fireEvent.click(screen.getByText("+ Новая задача"));
  fireEvent.change(screen.getByLabelText("Название"), {
    target: { value: "New task" },
  });
  fireEvent.click(screen.getByText("Сохранить"));
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  fireEvent.click(screen.getByText("Завершённые"));
  fireEvent.click(screen.getByText("Вернуть в работу"));
  await waitFor(() =>
    expect(calls.some((c) => c[1].body?.includes("task.reopen"))).toBe(true),
  );
  fireEvent.click(screen.getByText("Проекты"));
  fireEvent.change(screen.getByLabelText("Название проекта"), {
    target: { value: "New project" },
  });
  fireEvent.change(screen.getByLabelText("Родительский проект"), {
    target: { value: "p" },
  });
  fireEvent.click(screen.getByText("Создать"));
  await waitFor(() =>
    expect(calls.some((c) => c[1].body?.includes("project.create"))).toBe(true),
  );
  fireEvent.click(screen.getByText("Work", { selector: "button" }));
  expect(screen.getByText("Important task")).toBeInTheDocument();
  fireEvent.click(screen.getByText("Статистика"));
  await screen.findByText("Динамика по дням");
  fireEvent.click(screen.getByText("☾ Тёмная тема"));
  expect(document.documentElement.dataset.theme).toBe("dark");
  fireEvent.click(screen.getByText("☀ Светлая тема"));
  fireEvent.click(screen.getByText("Выйти"));
  await screen.findByText("С возвращением");
});
it("plans and renders the day", async () => {
  workspace.plans = [
    {
      id: "plan",
      date: localDate(),
      status: "active",
      items: [{ taskId: "t", allocatedMinutes: null, position: 0 }],
      mainOccurrenceId: "t",
      frogOccurrenceId: null,
    },
  ];
  app("/day");
  await screen.findByText("План дня");
  expect(screen.getAllByText("Work / Important task").length).toBe(2);
});
it("adds first task to today and preserves conflict", async () => {
  app("/inbox");
  await screen.findByText("+ Сегодня");
  fireEvent.click(screen.getByText("+ Сегодня"));
  await waitFor(() =>
    expect(calls.some((c) => c[1].body?.includes("plan.draft.save"))).toBe(
      true,
    ),
  );
  fail = true;
  fireEvent.click(screen.getByText("✓ Завершить"));
  await screen.findByRole("alert");
  fireEvent.click(screen.getByText("Обновить данные"));
  await waitFor(() =>
    expect(screen.queryByRole("alert")).not.toBeInTheDocument(),
  );
});
it("supports setup and login failures", async () => {
  authenticated = false;
  setup = true;
  app();
  await screen.findByText("Первый запуск");
  fireEvent.change(screen.getByLabelText("Пароль"), {
    target: { value: "test-password-long" },
  });
  fail = true;
  fireEvent.click(screen.getByText("Создать пространство"));
  await screen.findByRole("alert");
  fail = false;
  fireEvent.click(screen.getByText("Создать пространство"));
  await screen.findByText("План дня");
});
it("opens a direct task route and closes to inbox", async () => {
  app("/tasks/t");
  await screen.findByRole("dialog");
  fireEvent.click(screen.getByText("Отмена"));
  await screen.findByLabelText("Поиск задач");
});
it("settings manage consent, pairing, revocation and work hours", async () => {
  const save = vi.fn().mockResolvedValue(undefined);
  render(<Settings workspace={workspace} save={save} />);
  await screen.findByText("MacBook");
  fireEvent.change(screen.getByLabelText("Резерв, %"), {
    target: { value: 30 },
  });
  fireEvent.change(screen.getByLabelText("Начало дня"), {
    target: { value: "08:00" },
  });
  fireEvent.click(screen.getByText("Сохранить настройки"));
  await waitFor(() => expect(save).toHaveBeenCalled());
  fireEvent.click(screen.getByLabelText("Разрешить хранение дневника"));
  await waitFor(() =>
    expect(calls.some((c) => c[1].body?.includes('"enabled":true'))).toBe(true),
  );
  fireEvent.click(screen.getByText("Получить код сопряжения"));
  await screen.findByText("pair-code");
  fireEvent.click(screen.getByText("Отозвать доступ"));
  await waitFor(() =>
    expect(calls.some((c) => c[0].endsWith("/mac/revoke"))).toBe(true),
  );
});
it("wellbeing keeps unknown and explicit confirmation", async () => {
  const save = vi.fn().mockResolvedValue(undefined);
  render(<Wellbeing date="2026-10-05" workspace={workspace} save={save} />);
  expect(screen.getByLabelText("Настроение")).toHaveValue("unknown");
  fireEvent.change(screen.getByLabelText("Настроение"), {
    target: { value: "good" },
  });
  fireEvent.change(screen.getByLabelText("Заметка"), {
    target: { value: "Walk" },
  });
  fireEvent.click(screen.getByLabelText("Подтверждаю введённое состояние"));
  fireEvent.click(screen.getByText("Сохранить запись"));
  await screen.findByText("Состояние сохранено");
  expect(save).toHaveBeenCalledWith(
    "wellbeing.save",
    expect.objectContaining({
      wellbeing: {
        mood: "good",
        energy: "unknown",
        stress: "unknown",
        confirm: true,
      },
    }),
  );
  fireEvent.change(screen.getByLabelText("Дата"), {
    target: { value: "2026-10-06" },
  });
  expect(
    screen.getByLabelText("Подтверждаю введённое состояние"),
  ).not.toBeChecked();
  save.mockRejectedValue(new Error("Consent required"));
  fireEvent.click(screen.getByText("Сохранить запись"));
  await screen.findByText("Consent required");
});
it("saves habit counters with independent explicit confirmation", async () => {
  const save = vi.fn().mockResolvedValue(undefined);
  render(<Wellbeing date="2026-10-05" workspace={workspace} save={save} />);
  fireEvent.click(screen.getByText("Вредные привычки"));
  fireEvent.change(screen.getByLabelText("Сигареты за день, шт."), {
    target: { value: "0" },
  });
  fireEvent.change(screen.getByLabelText("Алкоголь за день, условных порций"), {
    target: { value: "2" },
  });
  fireEvent.click(
    screen.getByLabelText("Подтверждаю значения вредных привычек за день"),
  );
  fireEvent.click(screen.getByText("Сохранить запись"));
  await waitFor(() =>
    expect(save).toHaveBeenCalledWith(
      "wellbeing.save",
      expect.objectContaining({
        habits: { cigarettes: 0, alcoholPortions: 2, confirm: true },
        wellbeing: expect.objectContaining({ confirm: false }),
      }),
    ),
  );
});

it("shows the supplied logo, slogan and package version and tracks the active tab title", async () => {
  const { version } = await import("../package.json");
  app("/day");
  await screen.findByText("Spotter - личное рабочее пространство");
  expect(screen.getByAltText("spotter")).toHaveAttribute("src", "/logo.svg");
  expect(screen.getByText("Версия " + version)).toBeInTheDocument();
  expect(document.title).toBe("spotter - Сейчас");
  for (const [label, title] of [
    ["Входящие 1", "Входящие"],
    ["Планирование", "Планирование"],
    ["Проекты", "Проекты"],
    ["Статистика", "Статистика"],
    ["Моё состояние", "Моё состояние"],
    ["Настройки", "Настройки"],
    ["Сейчас", "Сейчас"],
  ]) {
    fireEvent.click(
      await within(screen.getByRole("navigation")).findByRole("link", {
        name: new RegExp(label + "$"),
      }),
    );
    await waitFor(() => expect(document.title).toBe("spotter - " + title));
  }
});
