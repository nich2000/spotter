import { ProjectTaskTree } from "./ProjectTaskTree";
import { projectPath, taskPath } from "./projectPaths";
import {
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type PointerEvent,
} from "react";
import { APIError, localDate, type Workspace, type Task } from "./api";
import {
  addDays,
  dayNumber,
  displayDate,
  displayInstant,
  visibleInterval,
} from "./calendar";
import {
  deadlineDate,
  scheduleExceedsDeadline,
  moveSchedule,
  scheduleDates,
  type Scheduled,
} from "./scheduling";
import { freezeCommand, type CommandRequest } from "./command";
import { projectScope } from "./projectScope";

export function scheduleLabel(s: Scheduled) {
  return s.kind === "date_range"
    ? displayDate(s.startDate) +
        (s.endDate ? " — " + displayDate(s.endDate) : " · без окончания")
    : displayInstant(s.startAt, s.zone) +
        (s.endAt
          ? " — " + displayInstant(s.endAt, s.zone)
          : " · без окончания");
}
export function planningTasks(w: Workspace, project: string) {
  const scope = projectScope(w.projects, project);
  return w.tasks.filter(
    (t) =>
      (t.lifecycleState !== "done" || (t.relatedTaskIds?.length ?? 0) > 0) &&
      (!project || scope.has(t.projectId ?? "")),
  );
}
type Props = {
  workspace: Workspace;
  revision?: number;
  query?: string;
  open: (t: Task, proposal?: Scheduled) => void;
  commit?: (c: CommandRequest) => Promise<any>;
  refresh?: () => Promise<void>;
};
type Gesture = {
  task: Task;
  original: Scheduled;
  proposal: Scheduled;
  x: number;
  unit: number;
  resize: boolean;
  revision: number;
  delta: number;
  invalid: boolean;
};
type Pending = {
  body: CommandRequest;
  task: Task;
  proposal: Scheduled;
  conflict: boolean;
};
export function Planning({
  workspace,
  revision = 0,
  query = "",
  open,
  commit,
  refresh,
}: Props) {
  const today = localDate(workspace.settings.zone),
    params = new URLSearchParams(query);
  const [start, setStart] = useState(today),
    [days, setDays] = useState(14),
    [project, setProject] = useState(params.get("project") ?? ""),
    [selected, setSelected] = useState(params.get("task") ?? "");
  const [preview, setPreview] = useState<{ id: string; s: Scheduled } | null>(
      null,
    ),
    [message, setMessage] = useState(""),
    [pending, setPending] = useState<Pending | null>(null),
    [busy, setBusy] = useState(false);
  const gesture = useRef<Gesture | null>(null),
    suppress = useRef(false);
  useEffect(() => {
    const p = new URLSearchParams(query);
    setProject(p.get("project") ?? "");
    const id = p.get("task") ?? "";
    setSelected(id);
    const t = workspace.tasks.find((t) => t.id === id);
    if (t?.scheduled) setStart(scheduleDates(t.scheduled)[0]);
    else if (t?.deadline) setStart(deadlineDate(t.deadline));
  }, [query]);
  useEffect(() => {
    const cancel = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        gesture.current = null;
        setPreview(null);
        setMessage("Перенос отменён");
      }
    };
    window.addEventListener("keydown", cancel);
    return () => window.removeEventListener("keydown", cancel);
  }, []);
  async function persist(p: Pending) {
    if (!commit) return;
    setBusy(true);
    setPending(p);
    try {
      const r = await commit(p.body);
      setPending(null);
      setMessage(
        r?.warnings?.includes("SCHEDULE_EXCEEDS_DEADLINE")
          ? "Сохранено. План выходит за дедлайн."
          : "Сроки сохранены",
      );
    } catch (e) {
      const conflict = e instanceof APIError && e.status === 409;
      setPending({ ...p, conflict });
      setMessage((e as Error).message);
      if (conflict) await refresh?.();
    } finally {
      setBusy(false);
    }
  }
  function begin(e: PointerEvent<HTMLButtonElement>, t: Task) {
    if (busy || pending || e.button !== 0 || !t.scheduled) return;
    const width = e.currentTarget.parentElement!.getBoundingClientRect().width;
    if (!width) return;
    const resize = (e.target as HTMLElement).dataset.resize === "true";
    gesture.current = {
      task: t,
      original: t.scheduled,
      proposal: t.scheduled,
      x: e.clientX,
      unit: width / days,
      resize,
      revision,
      delta: 0,
      invalid: false,
    };
    suppress.current = false;
    e.currentTarget.setPointerCapture(e.pointerId);
  }
  function move(e: PointerEvent<HTMLButtonElement>) {
    const g = gesture.current;
    if (!g) return;
    const delta = Math.round((e.clientX - g.x) / g.unit);
    g.delta = delta;
    suppress.current = suppress.current || delta !== 0;
    try {
      g.proposal = moveSchedule(g.original, delta, g.resize);
      g.invalid = false;
      setPreview({ id: g.task.id, s: g.proposal });
      setMessage(scheduleLabel(g.proposal));
    } catch (e) {
      g.invalid = true;
      setPreview(null);
      setMessage((e as Error).message);
    }
  }
  function finish() {
    const g = gesture.current;
    gesture.current = null;
    setPreview(null);
    if (g && g.delta && !g.invalid)
      void persist({
        body: freezeCommand(
          "task.save",
          { taskId: g.task.id, patch: { scheduled: g.proposal } },
          g.revision,
        ),
        task: g.task,
        proposal: g.proposal,
        conflict: false,
      });
  }
  const tasks = planningTasks(workspace, project);
  const edit = (t: Task) => {
    setSelected(t.id);
    open(t);
  };
  const row = (t: Task, depth: number) => {
    const s = preview?.id === t.id ? preview.s : t.scheduled;
    const range = s ? scheduleDates(s) : null,
      bar = range
        ? visibleInterval(range[0], range[1] ?? range[0], start, days)
        : null;
    const dd = t.deadline ? deadlineDate(t.deadline) : null;
    const deadline = dd ? visibleInterval(dd, dd, start, days) : null;
    const state =
      t.lifecycleState === "done"
        ? "Завершена"
        : t.workMode === "waiting"
          ? "Ожидание"
          : t.workMode === "active"
            ? "В работе"
            : "Запланировано";
    return (
      <div
        className={"planning-row " + (selected === t.id ? "selected" : "")}
        key={t.id}
      >
        <div
          className="planning-label"
          style={{ paddingLeft: 12 + depth * 16 }}
        >
          <div>
            <button onClick={() => edit(t)} title={t.title}>
              {t.title}
            </button>
            <small>
              {s ? scheduleLabel(s) : t.deadline ? "Только дедлайн" : "Без дат"}
              {t.deadline
                ? " · ◆ " +
                  (t.deadline.kind === "date"
                    ? displayDate(t.deadline.date)
                    : displayInstant(t.deadline.at, t.deadline.zone))
                : ""}
              {t.relatedTaskIds?.length
                ? " · ↔ " + t.relatedTaskIds.length
                : ""}
            </small>
            {s && t.deadline && scheduleExceedsDeadline(s, t.deadline) && (
              <small className="schedule-warning">
                План выходит за дедлайн
              </small>
            )}
            {workspace.plans
              .filter((p) => p.items.some((i) => i.taskId === t.id))
              .map((p) => (
                <small key={p.id}>В плане {displayDate(p.date)}</small>
              ))}
          </div>
          <small>
            {t.estimateMinutes == null
              ? "Без оценки"
              : t.estimateMinutes + " мин"}
          </small>
        </div>
        <div className="planning-lane">
          {bar && s && (
            <button
              className={
                "planning-bar " +
                (range![1] ? "" : "start-only ") +
                (t.workMode === "waiting" ? "wait " : "") +
                (t.lifecycleState === "done" ? "done" : "")
              }
              style={{
                left: bar.left + "%",
                width: range![1] ? bar.width + "%" : undefined,
              }}
              aria-label={t.title + ": " + scheduleLabel(s)}
              title={scheduleLabel(s) + " · " + s.zone}
              onClick={() => {
                if (!suppress.current) setSelected(t.id);
              }}
              onPointerDown={(e) => begin(e, t)}
              onPointerMove={move}
              onPointerUp={finish}
              onPointerCancel={() => {
                gesture.current = null;
                setPreview(null);
                setMessage("Перенос отменён");
              }}
              onDoubleClick={() => {
                if (!suppress.current) edit(t);
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  edit(t);
                }
              }}
            >
              {range![1] ? state : "●"}
              {range![1] && (
                <span
                  className="planning-handle"
                  data-resize="true"
                  aria-hidden="true"
                />
              )}
            </button>
          )}
          {deadline && (
            <span
              className="planning-deadline"
              title={"Дедлайн " + displayDate(dd!)}
              style={{ left: deadline.left + deadline.width / 2 + "%" }}
            >
              ◆
            </span>
          )}
          {(s || t.deadline) && !bar && !deadline && (
            <span className="outside">Вне периода</span>
          )}
          {s && !range![1] && (
            <button className="assign-end" onClick={() => edit(t)}>
              Назначить окончание
            </button>
          )}
          {!s && (
            <button className="assign-end" onClick={() => edit(t)}>
              {t.deadline ? "Назначить начало" : "Назначить даты"}
            </button>
          )}
        </div>
      </div>
    );
  };
  return (
    <section className="planning">
      <div className="toolbar">
        <div>
          <small>Сроки задач и независимые дедлайны</small>
        </div>
        <div className="planning-navigation">
          <button
            aria-label="Предыдущий период"
            onClick={() => setStart(addDays(start, -days))}
          >
            ←
          </button>
          <button onClick={() => setStart(today)}>Сегодня</button>
          <button
            aria-label="Следующий период"
            onClick={() => setStart(addDays(start, days))}
          >
            →
          </button>
        </div>
      </div>
      <div className="toolbar">
        <select
          aria-label="Фильтр проектов"
          value={project}
          onChange={(e) => {
            setProject(e.target.value);
          }}
        >
          <option value="">Все проекты</option>
          {workspace.projects.map((p) => (
            <option key={p.id} value={p.id}>
              {projectPath(workspace.projects, p.id)}
            </option>
          ))}
        </select>
        <label>
          Масштаб{" "}
          <select
            value={days}
            onChange={(e) => setDays(Number(e.target.value))}
          >
            {[14, 30, 90, 365].map((n) => (
              <option value={n} key={n}>
                {n} дней
              </option>
            ))}
          </select>
        </label>
        <small>
          {displayDate(start)} — {displayDate(addDays(start, days - 1))}
        </small>
      </div>
      <div className="planning-scroll">
        <div
          className="planning-board"
          style={{ "--day-width": 100 / days + "%" } as CSSProperties}
        >
          <div className="planning-row">
            <div className="planning-label muted">Задача / трудоёмкость</div>
            <div
              className="planning-days"
              style={{ gridTemplateColumns: "repeat(" + days + ",1fr)" }}
            >
              {Array.from({ length: days }, (_, i) => {
                const d = addDays(start, i);
                return (
                  <span
                    key={d}
                    title={displayDate(d)}
                    className={d === today ? "today" : ""}
                  >
                    {days <= 30
                      ? new Date(d + "T12:00:00Z").toLocaleDateString("ru", {
                          weekday: "short",
                          timeZone: "UTC",
                        })
                      : ""}
                    <b>
                      {days <= 30 || i % Math.ceil(days / 14) === 0
                        ? d.slice(8)
                        : ""}
                    </b>
                  </span>
                );
              })}
            </div>
          </div>
          <ProjectTaskTree
            projects={workspace.projects}
            tasks={tasks}
            rootId={project}
            resetKey={query}
            timeline
            renderTask={row}
          />
        </div>
      </div>
      <div className="planning-foot">
        <small>
          Полоса — плановый интервал · ● только начало · ◆ дедлайн · минуты
          слева — трудоёмкость
        </small>
        <small>Двойной клик или Enter — карточка задачи</small>
      </div>
      <p role="status">
        {message ||
          "Перетаскивайте полосу или её правый край. Для точного ввода и сенсорного экрана откройте карточку по названию."}
      </p>
      {pending && (
        <div className="panel">
          <p>
            {taskPath(workspace.projects, pending.task)}: черновик{" "}
            {scheduleLabel(pending.proposal)}
          </p>
          {pending.conflict ? (
            <>
              <p>
                Данные изменились. Актуальные сроки:{" "}
                {workspace.tasks.find((t) => t.id === pending.task.id)
                  ?.scheduled
                  ? scheduleLabel(
                      workspace.tasks.find((t) => t.id === pending.task.id)!
                        .scheduled!,
                    )
                  : "без расписания"}
                . Сравните их с черновиком перед сохранением.
              </p>
              <button
                disabled={busy}
                onClick={() => {
                  open(
                    workspace.tasks.find((t) => t.id === pending.task.id) ??
                      pending.task,
                    pending.proposal,
                  );
                  setPending(null);
                }}
              >
                Открыть черновик в карточке
              </button>
            </>
          ) : (
            <button disabled={busy} onClick={() => void persist(pending)}>
              Повторить тот же запрос
            </button>
          )}
          <button
            disabled={busy}
            onClick={() => {
              setPending(null);
              setMessage("Черновик отменён");
            }}
          >
            Отменить черновик
          </button>
        </div>
      )}
    </section>
  );
}
