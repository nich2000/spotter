import { ProjectTaskTree, type TreeCardOptions } from "./ProjectTaskTree";
import { projectPath, taskPath } from "./projectPaths";
import { useState, useEffect, ReactNode } from "react";
import { Task, Workspace, Plan, quadrant, request } from "./api";
type Command = (name: string, payload: unknown) => Promise<void>;
const zones = [
  ["q1", "Важно и срочно", "Решить в первую очередь"],
  ["q2", "Важно, не срочно", "Защитить время и запланировать"],
  ["q3", "Срочно, не важно", "Передать или выделить короткое окно"],
  ["q4", "Не важно, не срочно", "Отложить или отказаться осознанно"],
];
export function Inbox({
  workspace,
  command,
  card,
}: {
  workspace: Workspace;
  command: Command;
  card: (t: Task, options?: TreeCardOptions) => ReactNode;
}) {
  const [query, setQuery] = useState(""),
    [title, setTitle] = useState(""),
    [error, setError] = useState(""),
    [sources, setSources] = useState<any[]>([]);
  async function refresh() {
    try {
      const r = await request("/sources");
      setSources(r.items ?? []);
    } catch (e) {
      setError((e as Error).message);
    }
  }
  useEffect(() => {
    void refresh();
  }, []);
  const tasks = workspace.tasks.filter((t) =>
    taskPath(workspace.projects, t)
      .toLocaleLowerCase()
      .includes(query.toLocaleLowerCase()),
  );
  const candidates = workspace.candidates ?? [];
  async function run(name: string, payload: unknown) {
    try {
      setError("");
      await command(name, payload);
    } catch (e) {
      setError((e as Error).message);
    }
  }
  function drop(e: React.DragEvent, zone: string) {
    e.preventDefault();
    const id = e.dataTransfer.getData("text/spotter-task");
    if (
      !workspace.tasks.some((t) => t.id === id && t.lifecycleState !== "done")
    )
      return;
    void run("task.triage", {
      taskId: id,
      triage:
        zone === "unreviewed"
          ? { action: "require" }
          : {
              action: "complete",
              importance: ["q1", "q2"].includes(zone) ? "yes" : "no",
              urgency: ["q1", "q3"].includes(zone) ? "yes" : "no",
            },
    });
  }
  const rows = (zone: string) =>
    tasks
      .filter((t) => t.lifecycleState !== "done" && quadrant(t) === zone)
      .map((t) => card(t));
  return (
    <>
      <div className="source-strip">
        {[
          ["calendar", "Календарь"],
          ["reminders", "Напоминания"],
          ["notes", "Заметки"],
          ["mail", "Почта"],
        ].map(([key, label]) => (
          <span key={key}>
            {label} ·{" "}
            {sources.some((s) => s.name === key && s.snapshot.ok)
              ? "получен снимок"
              : sources.some((s) => s.name === key)
                ? "ошибка сбора"
                : "нет снимка"}
          </span>
        ))}
        <button onClick={() => void refresh()}>Обновить статусы</button>
      </div>
      <form
        className="capture"
        onSubmit={async (e) => {
          e.preventDefault();
          try {
            await command("task.create", { title: title.trim() });
            setTitle("");
            setError("");
          } catch (e) {
            setError((e as Error).message);
          }
        }}
      >
        <input
          aria-label="Быстро во Входящие"
          placeholder="Записать мысль, задачу или заметку…"
          required
          maxLength={500}
          value={title}
          onChange={(e) => setTitle(e.target.value)}
        />
        <button className="primary">Добавить во Входящие</button>
      </form>
      <input
        className="inbox-search"
        aria-label="Поиск задач"
        placeholder="Найти задачу…"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      <div className="inbox-workspace">
        <div className="inbox-intake">
          <details
            className="intake-block"
            open
            onDragOver={(e) => e.preventDefault()}
            onDrop={(e) => drop(e, "unreviewed")}
          >
            <summary>
              Неразобранное <small>{rows("unreviewed").length}</small>
            </summary>
            <p>Новые записи и повторный разбор.</p>
            <ProjectTaskTree
              projects={workspace.projects}
              tasks={tasks.filter(
                (t) =>
                  t.lifecycleState !== "done" && quadrant(t) === "unreviewed",
              )}
              renderTask={(t) => card(t, { inTree: true, draggable: false })}
            />
          </details>
          <details className="intake-block" open>
            <summary>
              Предложения системы{" "}
              <small>
                {candidates.filter((c) => c.state === "pending").length}
              </small>
            </summary>
            <p>Предложения из подключённых источников.</p>
            <div className="compact-list">
              {candidates
                .filter((c) => c.state === "pending")
                .map((c) => (
                  <article className="candidate" key={c.id}>
                    <span>{c.sourceId} · предложение</span>
                    <strong>{c.proposedTitle}</strong>
                    <div>
                      <button
                        onClick={() =>
                          void run("source.accept", {
                            candidateId: c.id,
                            candidateVersion: c.candidateVersion,
                            sourceSnapshotId: c.sourceSnapshotId,
                          })
                        }
                      >
                        Принять
                      </button>
                      <button
                        onClick={() =>
                          void run("source.dismiss", {
                            candidateId: c.id,
                            candidateVersion: c.candidateVersion,
                            sourceSnapshotId: c.sourceSnapshotId,
                          })
                        }
                      >
                        Пропустить
                      </button>
                    </div>
                  </article>
                ))}
            </div>
          </details>
        </div>
        <section>
          <h2>Что важнее?</h2>
          <p>
            Перетащите строку в квадрант. Нажмите название или ⋯ для деталей.
          </p>
          <div className="matrix">
            {zones.map(([key, label, hint]) => (
              <section
                className={"quadrant " + key}
                key={key}
                onDragOver={(e) => e.preventDefault()}
                onDrop={(e) => drop(e, key)}
                aria-label={label}
              >
                <h3>
                  {label} <small>{rows(key).length}</small>
                </h3>
                <p>{hint}</p>
                <div className="compact-list">{rows(key)}</div>
              </section>
            ))}
          </div>
          <p className="legend">
            Важность — вклад в цель · Срочность — срок и последствия задержки
          </p>
        </section>
      </div>
      <div className="inbox-bottom">
        <p>Внешние записи — предложения. Оригиналы не изменяются.</p>
        <details>
          <summary>
            Пропущенные предложения (
            {candidates.filter((c) => c.state === "dismissed").length})
          </summary>
          {candidates
            .filter((c) => c.state === "dismissed")
            .map((c) => (
              <div className="candidate" key={c.id}>
                {c.proposedTitle}
                <button
                  onClick={() =>
                    void run("source.restore", {
                      candidateId: c.id,
                      candidateVersion: c.candidateVersion,
                      sourceSnapshotId: c.sourceSnapshotId,
                    })
                  }
                >
                  Вернуть в предложения системы
                </button>
              </div>
            ))}
        </details>
        <details>
          <summary>Завершённые</summary>
          <div className="compact-list">
            {tasks
              .filter((t) => t.lifecycleState === "done")
              .map((t) => card(t))}
          </div>
        </details>
      </div>
    </>
  );
}
export function Workday({
  workspace,
  plan,
  card,
  open,
}: {
  workspace: Workspace;
  plan?: Plan;
  card: (t: Task, options?: TreeCardOptions) => ReactNode;
  open: (t: Task) => void;
}) {
  const tasks = workspace.tasks.filter((t) =>
    plan?.items.some((i) => i.taskId === t.id),
  );
  const focus =
    tasks.find((t) => t.workMode === "active") ??
    tasks.find((t) => t.id === plan?.mainOccurrenceId);
  const groups = [
    ["Не начато", tasks.filter((t) => t.lifecycleState === "not_started")],
    [
      "В процессе",
      tasks.filter(
        (t) => t.lifecycleState === "in_progress" && t.workMode !== "waiting",
      ),
    ],
    [
      "Ожидание",
      tasks.filter(
        (t) => t.lifecycleState === "in_progress" && t.workMode === "waiting",
      ),
    ],
    ["Готово", tasks.filter((t) => t.lifecycleState === "done")],
  ] as const;
  return (
    <>
      <div className="focus-layout">
        <section className="panel">
          <span className="eyebrow">
            {focus?.context === "away" ? "Вне компьютера" : "За компьютером"} ·{" "}
            {projectPath(workspace.projects, focus?.projectId ?? null)}
          </span>
          <h2>
            {focus
              ? taskPath(workspace.projects, focus)
              : "Выберите результат дня"}
          </h2>
          <p>
            {focus?.resume ??
              "Добавьте задачу из Входящих, чтобы собрать план."}
          </p>
          {focus?.subtasks.map((s) => (
            <label className="check" key={s.id}>
              <input type="checkbox" checked={s.done} readOnly />
              {s.title}
            </label>
          ))}
          {focus && (
            <button onClick={() => open(focus)}>Открыть карточку задачи</button>
          )}
        </section>
        <section className="panel timer">
          <span className="eyebrow">Фокус и перерыв</span>
          <div className="clock">— : —</div>
          <p>Таймер недоступен до согласования правил учёта времени.</p>
          <button disabled>Начать фокус</button>
        </section>
      </div>
      <div className="day-summary">
        <span>
          В плане <strong>{tasks.length} / 3</strong>
        </span>
        <span>
          Доступное время <strong>не рассчитано</strong>
        </span>
        <span>
          Резерв <strong>{workspace.settings.reservePercent}%</strong>
        </span>
      </div>
      <div className="section-heading">
        <h2>План дня</h2>
        <a href="/inbox">Выбрать из Входящих →</a>
      </div>
      <div className="day-board">
        {groups.map(([name, items]) => (
          <section key={name}>
            <h3>
              {name} <small>{items.length}</small>
            </h3>
            {items.map((t) => card(t))}
          </section>
        ))}
      </div>
      {!plan && <p>День ещё не спланирован</p>}
      <details className="panel">
        <summary>История планов и результатов</summary>
        <p>
          Закрытие дня и исторические результаты ожидают согласования правил.
          Текущий план сохраняется на сервере.
        </p>
      </details>
    </>
  );
}
export function ProjectTree({
  workspace,
  command,
  card,
}: {
  workspace: Workspace;
  command: Command;
  card: (t: Task, options?: TreeCardOptions) => ReactNode;
}) {
  const [name, setName] = useState(""),
    [parent, setParent] = useState(""),
    [selected, setSelected] = useState(""),
    [error, setError] = useState("");
  return (
    <>
      <p>
        Стрелки раскрывают ветки, названия выбирают проект. Сводки включают
        вложенные ветки.
      </p>
      <form
        className="capture"
        onSubmit={async (e) => {
          e.preventDefault();
          try {
            await command("project.create", { name, parentId: parent || null });
            setName("");
            setError("");
          } catch (e) {
            setError((e as Error).message);
          }
        }}
      >
        <input
          aria-label="Название проекта"
          placeholder="Новый проект / подпроект"
          value={name}
          required
          onChange={(e) => setName(e.target.value)}
        />
        <select
          aria-label="Родительский проект"
          value={parent}
          onChange={(e) => setParent(e.target.value)}
        >
          <option value="">Корневой проект</option>
          {workspace.projects.map((p) => (
            <option key={p.id} value={p.id}>
              {projectPath(workspace.projects, p.id)}
            </option>
          ))}
        </select>
        <button className="primary">Создать</button>
      </form>
      {error && <p role="alert">{error}</p>}
      <div className="project-tree panel">
        <ProjectTaskTree
          projects={workspace.projects}
          tasks={workspace.tasks}
          selected={selected}
          onSelect={(id) => {
            setSelected(id);
            setParent(id);
          }}
          onMove={async (taskId, projectId) => {
            await command("task.move", { taskId, projectId });
          }}
          projectActions={(p) => (
            <a href={"/planning?project=" + encodeURIComponent(p.id)}>
              Показать на диаграмме
            </a>
          )}
          renderTask={(t) => card(t, { inTree: true, draggable: true })}
        />
      </div>
    </>
  );
}
export function Statistics() {
  return (
    <>
      <div className="toolbar">
        <label>
          Период
          <input
            type="month"
            defaultValue={new Date().toISOString().slice(0, 7)}
          />
        </label>
      </div>
      <div className="report-metrics">
        {[
          "Завершено задач",
          "Измеренный фокус",
          "Главный результат дня",
          "Начато и не завершено",
        ].map((s) => (
          <section className="panel" key={s}>
            <span>{s}</span>
            <strong>—</strong>
            <small>Исторический расчёт недоступен</small>
          </section>
        ))}
      </div>
      <section className="panel">
        <h2>Динамика по дням</h2>
        <div className="chart-empty">Нет рассчитанных данных за период</div>
        <p>Измеренное время · Указано вручную, приблизительно</p>
        <small>Отсутствие записей времени не означает отсутствие работы.</small>
      </section>
      <section className="panel">
        <h2>Движение по проектам</h2>
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                {["Проект", "Добавлено", "Завершено", "Открыто", "Фокус"].map(
                  (t) => (
                    <th key={t}>{t}</th>
                  ),
                )}
              </tr>
            </thead>
            <tbody>
              <tr>
                <td colSpan={5}>
                  Исторические показатели ожидают согласования правил расчёта.
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
      <div className="settings-grid">
        <section className="panel">
          <h2>Насколько реалистичен план</h2>
          <p>Один день учитывается один раз. Расчёт недоступен.</p>
        </section>
        <section className="panel">
          <h2>На что обратить внимание</h2>
          <p>Нет рассчитанных наблюдений.</p>
        </section>
      </div>
    </>
  );
}
export { Planning } from "./Planning";
