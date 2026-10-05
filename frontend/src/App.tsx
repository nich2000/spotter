import { CalendarView } from "./CalendarView";
import { projectPath, taskPath } from "./projectPaths";
import type { TreeCardOptions } from "./ProjectTaskTree";
import { version as appVersion } from "../package.json";
import { sendCommand, freezeCommand, type CommandRequest } from "./command";
import type { Scheduled } from "./scheduling";
import { useEffect, useRef, useState } from "react";
import { NavLink, useLocation, useNavigate } from "react-router-dom";
import {
  APIError,
  localDate,
  post,
  quadrant,
  request,
  Task,
  Workspace,
} from "./api";
import { TaskEditor } from "./TaskEditor";
import {
  Inbox,
  Workday,
  ProjectTree,
  Statistics,
  Planning,
} from "./MockupViews";
import { Settings, Wellbeing } from "./Settings";
const navigation = [
  ["/day", "◉", "Сейчас"],
  ["/inbox", "▤", "Входящие"],
  ["/calendar", "", "Календарь"],
  ["/planning", "", "Планирование"],
  ["/projects", "▧", "Проекты"],
  ["/statistics", "▥", "Статистика"],
  ["/wellbeing", "♡", "Моё состояние"],
  ["/settings", "⚙", "Настройки"],
];
const sections = [
  ["unreviewed", "Неразобранное"],
  ["q1", "Важно · срочно"],
  ["q2", "Важно · не срочно"],
  ["q3", "Не важно · срочно"],
  ["q4", "Не важно · не срочно"],
  ["done", "Завершённые"],
];
function Branding() {
  return (
    <>
      <img className="brand-logo" src="/logo.svg" alt="spotter" />
      <span className="brand-tagline">
        Spotter - личное рабочее пространство
      </span>
      <span className="brand-version">Версия {appVersion}</span>
    </>
  );
}
export function App() {
  const [session, setSession] = useState<{
    authenticated: boolean;
    setupRequired: boolean;
  } | null>(null);
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [revision, setRevision] = useState(0);
  const [error, setError] = useState("");
  const [editor, setEditor] = useState<Task | null | undefined>();
  const [editRevision, setEditRevision] = useState(0);
  const [editSchedule, setEditSchedule] = useState<Scheduled>();
  const editorRequest = useRef<CommandRequest | null>(null);
  const [theme, setTheme] = useState(
    localStorage.getItem("spotter-theme") ?? "light",
  );
  const generation = useRef(0);
  const requestGeneration = useRef(0);
  const location = useLocation();
  const navigate = useNavigate();
  const path = location.pathname;
  useEffect(() => {
    const active =
      path === "/inbox"
        ? "Входящие"
        : path.startsWith("/tasks/")
          ? "Задача"
          : (navigation.find((n) => n[0] === path)?.[2] ?? "Сейчас");
    document.title = "spotter - " + active;
  }, [path]);

  const date = localDate(workspace?.settings.zone);
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem("spotter-theme", theme);
  }, [theme]);
  async function load() {
    const gen = generation.current;
    const seq = ++requestGeneration.current;
    try {
      const data = await request("/workspace");
      if (gen === generation.current && seq === requestGeneration.current) {
        setWorkspace(data.data);
        setRevision(data.workspaceRevision);
      }
    } catch (e) {
      if (gen !== generation.current) return;
      setError((e as Error).message);
      if (e instanceof APIError && e.status === 401) {
        setSession({ authenticated: false, setupRequired: false });
        setWorkspace(null);
      }
    }
  }
  useEffect(() => {
    void request("/auth/session")
      .then(setSession)
      .catch((e) => setError(e.message));
  }, []);
  useEffect(() => {
    if (!session?.authenticated) return;
    void load();
    const stream = new EventSource("/api/v2/events");
    stream.addEventListener("reset", () => void load());
    const refresh = () => void load();
    window.addEventListener("focus", refresh);
    return () => {
      stream.close();
      window.removeEventListener("focus", refresh);
    };
  }, [session?.authenticated]);
  useEffect(() => {
    if (path.startsWith("/tasks/") && workspace && editor === undefined) {
      const task = workspace.tasks.find((t) => t.id === path.split("/")[2]);
      if (task) {
        setEditor(task);
        setEditRevision(revision);
      }
    }
  }, [path, workspace]);
  async function command(name: string, payload: unknown, expected = revision) {
    const body = {
      operationId: crypto.randomUUID(),
      expectedRevision: expected,
      command: name,
      payload,
    };
    const receipt = await post("/workspace/commands", body);
    if (receipt.warnings?.includes("SCHEDULE_EXCEEDS_DEADLINE"))
      setError("Сохранено. План выходит за дедлайн.");
    await load();
  }
  async function action(name: string, payload: unknown) {
    try {
      setError("");
      await command(name, payload);
    } catch (e) {
      setError((e as Error).message);
    }
  }
  async function commitFrozen(body: CommandRequest) {
    const receipt = await sendCommand(body);
    await load();
    return receipt;
  }
  async function saveEditor(name: string, payload: unknown) {
    // An unknown transport outcome must be retried before issuing another operation.
    const body =
      editorRequest.current ?? freezeCommand(name, payload, editRevision);
    editorRequest.current = body;
    try {
      const receipt = await commitFrozen(body);
      editorRequest.current = null;
      if (receipt.warnings?.includes("SCHEDULE_EXCEEDS_DEADLINE"))
        setError("Сохранено. План выходит за дедлайн.");
    } catch (e) {
      if (e instanceof APIError && e.status < 500) editorRequest.current = null;
      throw e;
    }
  }
  function open(task: Task | null, proposal?: Scheduled) {
    editorRequest.current = null;
    setEditSchedule(proposal);
    setEditor(task);
    setEditRevision(revision);
  }
  async function logout() {
    try {
      await post("/auth/logout", {});
      generation.current++;
      setWorkspace(null);
      setEditor(undefined);
      setSession({ authenticated: false, setupRequired: false });
    } catch (e) {
      setError((e as Error).message);
    }
  }
  if (!session)
    return <main className="loading">{error || "Подключение к Spotter…"}</main>;
  if (!session.authenticated)
    return (
      <Login
        setup={session.setupRequired}
        error={error}
        logged={() => {
          generation.current++;
          setSession({ authenticated: true, setupRequired: false });
          setError("");
        }}
      />
    );
  const tasks = workspace?.tasks ?? [];
  const projects = workspace?.projects ?? [];
  const plans = workspace?.plans ?? [];
  const plan = plans.find((p) => p.date === date && p.status === "active");
  const title = navigation.find((n) => n[0] === path)?.[2] ?? "Рабочий день";
  async function addToPlan(task: Task) {
    const items = [
      ...(plan?.items ?? []),
      {
        taskId: task.id,
        allocatedMinutes: task.estimateMinutes,
        position: plan?.items.length ?? 0,
      },
    ];
    await action(plan ? "plan.update" : "plan.draft.save", {
      ...(plan
        ? { planId: plan.id }
        : { date, zone: workspace!.settings.zone, activate: true }),
      items,
      mainOccurrenceId: plan?.mainOccurrenceId ?? task.id,
      frogOccurrenceId: plan?.frogOccurrenceId ?? null,
    });
  }
  function card(task: Task, options: TreeCardOptions = {}) {
    return (
      <article
        className="task"
        key={task.id}
        draggable={options.draggable ?? task.lifecycleState !== "done"}
        onDragStart={(e) =>
          e.dataTransfer.setData("text/spotter-task", task.id)
        }
      >
        <button className="task-title" title={taskPath(projects, task)} onClick={() => open(task)}>
          {options.inTree ? task.title : taskPath(projects, task)}
        </button>
        <div className="tags">
          <span>{projectPath(projects, task.projectId)}</span>
          <span>
            {task.estimateMinutes === null
              ? "Нужна оценка"
              : `${task.estimateMinutes} мин`}
          </span>
          <span>
            {task.lifecycleState === "done"
              ? "Завершена"
              : task.lifecycleState === "in_progress"
                ? "В процессе"
                : "Не начата"}
          </span>
        </div>
        {task.resume && <p>{task.resume}</p>}
        <details className="row-menu">
          <summary
            aria-label={
              "Действия: " +
              (options.inTree ? task.title : taskPath(projects, task))
            }
          >
            ⋯
          </summary>
          <div className="task-actions">
            <button
              onClick={() =>
                navigate("/planning?task=" + encodeURIComponent(task.id))
              }
            >
              Запланировать
            </button>
            {task.lifecycleState !== "done" ? (
              <>
                <button
                  onClick={() =>
                    task.subtasks.some((s) => !s.done)
                      ? open(task)
                      : void action("task.transition", {
                          taskId: task.id,
                          transition: {
                            to: "done",
                            completionActor: "self",
                            unfinishedSubtasksDecision: "keep_open",
                          },
                        })
                  }
                >
                  ✓ Завершить
                </button>
                {!plan?.items.some((i) => i.taskId === task.id) && (
                  <button onClick={() => void addToPlan(task)}>
                    + Сегодня
                  </button>
                )}
                <details>
                  <summary>Приоритет</summary>
                  {sections.slice(1, 5).map(([key, label], i) => (
                    <button
                      key={key}
                      onClick={() =>
                        void action("task.triage", {
                          taskId: task.id,
                          triage: {
                            action: "complete",
                            importance: i < 2 ? "yes" : "no",
                            urgency: i % 2 === 0 ? "yes" : "no",
                          },
                        })
                      }
                    >
                      {label}
                    </button>
                  ))}
                </details>
              </>
            ) : (
              <button
                onClick={() =>
                  void action("task.reopen", {
                    taskId: task.id,
                    completionEventId: task.completionEventId,
                  })
                }
              >
                Вернуть в работу
              </button>
            )}
          </div>
        </details>
      </article>
    );
  }
  return (
    <div className="shell">
      <aside className="sidebar">
        <a className="brand" href="/day">
          <Branding />
        </a>
        <nav>
          {navigation.map(([href, icon, label]) => (
            <NavLink key={href} to={href}>
              <span>{icon}</span>
              {label}
              {href === "/inbox" && (
                <b>
                  {
                    tasks.filter(
                      (t) =>
                        t.lifecycleState !== "done" &&
                        quadrant(t) === "unreviewed",
                    ).length
                  }
                </b>
              )}
            </NavLink>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <button
            onClick={() => setTheme(theme === "light" ? "dark" : "light")}
          >
            {theme === "light" ? "☾ Тёмная тема" : "☀ Светлая тема"}
          </button>
          <button onClick={() => void logout()}>Выйти</button>
          <small>
            Данные на вашем сервере
            <br />
            Версия 2 · локальный профиль
          </small>
        </div>
      </aside>
      <main className="main">
        <header className="page-header">
          <div>
            <span className="eyebrow">
              {new Date().toLocaleDateString("ru", {
                weekday: "long",
                day: "numeric",
                month: "long",
              })}
            </span>
            <h1>
              {path === "/inbox"
                ? "Разобрать входящее"
                : path === "/day" || path === "/"
                  ? "Сейчас в фокусе"
                  : path === "/statistics"
                    ? "Результаты и время"
                    : path === "/wellbeing"
                      ? "Как проходит день"
                      : title}
            </h1>
          </div>
          <button className="primary" onClick={() => open(null)}>
            + Новая задача
          </button>
        </header>
        {error && (
          <div className="error" role="alert">
            {error}
            <button
              onClick={() => {
                setError("");
                void load();
              }}
            >
              Обновить данные
            </button>
          </div>
        )}
        {!workspace ? (
          <p>Загрузка пространства…</p>
        ) : (
          <>
            {(path === "/day" ||
              path === "/" ||
              path.startsWith("/tasks/")) && (
              <Workday
                workspace={workspace}
                plan={plan}
                card={card}
                open={open}
              />
            )}
            {path === "/inbox" && (
              <Inbox workspace={workspace} command={command} card={card} />
            )}
            {path === "/projects" && (
              <ProjectTree
                workspace={workspace}
                command={command}
                card={card}
              />
            )}
            {path === "/statistics" && <Statistics />}
            {path === "/calendar" && <CalendarView workspace={workspace} revision={revision} open={open} commit={commitFrozen} refresh={load} />}
            {path === "/planning" && (
              <Planning
                workspace={workspace}
                revision={revision}
                query={location.search}
                open={open}
                commit={commitFrozen}
                refresh={load}
              />
            )}
            {path === "/wellbeing" && (
              <Wellbeing date={date} workspace={workspace} save={command} />
            )}
            {path === "/settings" && (
              <Settings workspace={workspace} save={command} />
            )}
          </>
        )}
      </main>
      {editor !== undefined && (
        <TaskEditor
          key={editor?.id ?? "new"}
          task={editor}
          projects={projects}
          tasks={workspace?.tasks}
          zone={workspace?.settings.zone}
          proposedSchedule={editSchedule}
          focusPlanning={path === "/planning"}
          save={saveEditor}
          resolveConflict={async () => {
            await load();
            const actual = await request("/workspace");
            setEditRevision(actual.workspaceRevision);
            return actual.data.tasks.find((t: Task) => t.id === editor?.id);
          }}
          close={() => {
            setEditor(undefined);
            if (path.startsWith("/tasks/")) navigate("/inbox");
          }}
        />
      )}
    </div>
  );
}
function Login({
  setup,
  error,
  logged,
}: {
  setup: boolean;
  error: string;
  logged: () => void;
}) {
  const [password, setPassword] = useState("");
  const [message, setMessage] = useState(error);
  const [busy, setBusy] = useState(false);
  return (
    <main className="login">
      <div className="login-art">
        <span className="brand">
          <Branding />
        </span>
        <h1>
          Меньше шума.
          <br />
          Больше внимания
          <br />
          важному.
        </h1>
        <p>
          Задачи, планы и личное состояние
          <br />в одном спокойном пространстве.
        </p>
      </div>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          try {
            await post(setup ? "/auth/setup" : "/auth/login", { password });
            logged();
          } catch (e) {
            setMessage((e as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        <span className="eyebrow">ВАШЕ ПРОСТРАНСТВО</span>
        <h2>{setup ? "Первый запуск" : "С возвращением"}</h2>
        <p>
          {setup
            ? "Задайте пароль владельца. Он защищает все данные и подключение устройств."
            : "Войдите, чтобы продолжить работу."}
        </p>
        <label>
          Пароль
          <input
            type="password"
            autoComplete={setup ? "new-password" : "current-password"}
            required
            minLength={setup ? 12 : 1}
            maxLength={72}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </label>
        {message && (
          <p role="alert" className="error">
            {message}
          </p>
        )}
        <button className="primary" disabled={busy}>
          {busy ? "Подождите…" : setup ? "Создать пространство" : "Войти"}
        </button>
        <small>Локальная установка · cookie-сессия</small>
      </form>
    </main>
  );
}
export function Empty({ title, text }: { title: string; text: string }) {
  return (
    <div className="empty">
      <span>◎</span>
      <h3>{title}</h3>
      <p>{text}</p>
    </div>
  );
}
