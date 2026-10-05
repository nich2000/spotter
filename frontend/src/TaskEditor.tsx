import { projectPath, taskPath } from "./projectPaths";
import { useState, useEffect, useRef } from "react";
import { APIError, Task, Project } from "./api";
import {
  SchedulingFields,
  scheduleDraft,
  schedulePatch,
} from "./SchedulingFields";
import type { Scheduled } from "./scheduling";
type Props = {
  resolveConflict?: () => Promise<Task | undefined>;
  focusPlanning?: boolean;
  tasks?: Task[];
  zone?: string;
  proposedSchedule?: Scheduled;
  task: Task | null;
  projects: Project[];
  save: (command: string, payload: unknown) => Promise<void>;
  close: () => void;
};
export function TaskEditor({
  task,
  projects,
  save,
  close,
  tasks = [],
  zone = "Europe/Moscow",
  proposedSchedule,
  resolveConflict,
  focusPlanning,
}: Props) {
  const [conflict, setConflict] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const [schedule, setSchedule] = useState(() =>
    scheduleDraft(proposedSchedule ?? task?.scheduled, task?.deadline, zone),
  );
  const [links, setLinks] = useState(task?.relatedTaskIds ?? []);
  const [title, setTitle] = useState(task?.title ?? "");
  const [description, setDescription] = useState(task?.description ?? "");
  const [resume, setResume] = useState(task?.resume ?? "");
  const [project, setProject] = useState(task?.projectId ?? "");
  const [estimate, setEstimate] = useState(
    task?.estimateMinutes?.toString() ?? "",
  );
  const [context, setContext] = useState(task?.context ?? "anywhere");
  const [complexity, setComplexity] = useState(task?.complexity ?? "");
  const [splittable, setSplittable] = useState(task?.splittable ?? true);
  const [multiDay, setMultiDay] = useState(task?.multiDay ?? false);
  const [comment, setComment] = useState("");
  const [subtasks, setSubtasks] = useState<
    Array<{
      id?: string;
      clientRef?: string;
      title: string;
      done: boolean;
      position: number;
    }>
  >(task?.subtasks ?? []);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const initialState =
    task?.lifecycleState === "in_progress"
      ? (task.workMode ?? "paused")
      : (task?.lifecycleState ?? "not_started");
  const [state, setState] = useState(initialState);
  const [remaining, setRemaining] = useState(
    task?.remainingMinutes?.toString() ?? "",
  );
  const [importance, setImportance] = useState(task?.importance ?? "unknown");
  const [urgency, setUrgency] = useState(task?.urgency ?? "unknown");
  const [reason, setReason] = useState(
    (task as any)?.waitingOn ?? (task as any)?.backgroundReason ?? "",
  );
  const originalCheck = (task as any)?.checkAt;
  const [checkAt, setCheckAt] = useState(
    originalCheck
      ? new Date(
          new Date(originalCheck).getTime() -
            new Date(originalCheck).getTimezoneOffset() * 60000,
        )
          .toISOString()
          .slice(0, 16)
      : "",
  );
  const [decision, setDecision] = useState("");
  const [discard, setDiscard] = useState(false);
  const dirty = useRef(!!proposedSchedule),
    root = useRef<HTMLElement>(null);
  const attemptClose = () => {
    if (busy) return;
    if (dirty.current) setDiscard(true);
    else close();
  };
  const closeRef = useRef(attemptClose);
  closeRef.current = attemptClose;
  useEffect(() => {
    const previous = document.activeElement as HTMLElement;
    if (focusPlanning) {
      const section = root.current?.querySelector("#task-planning");
      section?.scrollIntoView?.({ block: "start" });
      section?.querySelector("select")?.focus();
    }
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        closeRef.current();
      }
      if (e.key === "Tab") {
        const items = Array.from(
          root.current?.querySelectorAll<HTMLElement>(
            "button:not(:disabled),input:not(:disabled),select:not(:disabled),textarea:not(:disabled)",
          ) ?? [],
        );
        const first = items[0],
          last = items[items.length - 1];
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last?.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first?.focus();
        }
      }
    };
    document.addEventListener("keydown", handler);
    return () => {
      document.removeEventListener("keydown", handler);
      previous?.focus();
    };
  }, []);
  async function submit() {
    if (
      state === "done" &&
      initialState !== "done" &&
      subtasks.some((s) => !s.done) &&
      !decision
    ) {
      setError("Выберите, как завершить незавершённые подпункты");
      return;
    }

    setBusy(true);
    setError("");
    const patch = {
      title: title.trim(),
      description,
      resume,
      projectId: project || null,
      estimateMinutes: estimate ? Number(estimate) : null,
      context,
      complexity: complexity || null,
      multiDay,
      splittable,
      remainingMinutes: remaining ? Number(remaining) : null,
      importance,
      urgency,
    };
    try {
      if (
        proposedSchedule ||
        JSON.stringify(schedule) !==
          JSON.stringify(scheduleDraft(task?.scheduled, task?.deadline, zone))
      )
        Object.assign(patch, schedulePatch(schedule));
      if (JSON.stringify(links) !== JSON.stringify(task?.relatedTaskIds ?? []))
        Object.assign(patch, { relatedTaskIds: links });
      await save(task ? "task.save" : "task.create", {
        ...(task
          ? { taskId: task.id }
          : { title: patch.title, projectId: patch.projectId }),
        patch,
        ...(importance !== (task?.importance ?? "unknown") ||
        urgency !== (task?.urgency ?? "unknown")
          ? {
              triage:
                importance !== "unknown" && urgency !== "unknown"
                  ? { action: "complete", importance, urgency }
                  : { action: "require" },
            }
          : {}),
        ...(state !== initialState ||
        (["waiting", "background"].includes(state) &&
          (reason !==
            ((task as any)?.waitingOn ??
              (task as any)?.backgroundReason ??
              "") ||
            (checkAt ? new Date(checkAt).toISOString() : null) !==
              (originalCheck ? new Date(originalCheck).toISOString() : null)))
          ? state === "done"
            ? {
                transition: {
                  to: "done",
                  completionActor: "self",
                  unfinishedSubtasksDecision: decision || "keep_open",
                },
              }
            : initialState === "done"
              ? { reopen: { completionEventId: task?.completionEventId } }
              : {
                  transition: {
                    to: state === "not_started" ? "not_started" : "in_progress",
                    workMode: state === "not_started" ? null : state,
                    waitingOn: state === "waiting" ? reason : null,
                    backgroundReason: state === "background" ? reason : null,
                    checkAt: checkAt ? new Date(checkAt).toISOString() : null,
                  },
                }
          : {}),
        subtasks,
        commentsToAdd: comment.trim()
          ? [{ clientRef: "comment", text: comment.trim() }]
          : [],
      });
      close();
    } catch (e) {
      setConflict(e instanceof APIError && e.status === 409);
      setUncertain(
        e instanceof TypeError || (e instanceof APIError && e.status >= 500),
      );
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="scrim">
      <section
        ref={root}
        className="editor"
        role="dialog"
        aria-modal="true"
        aria-label={task ? "Редактирование задачи" : "Создание задачи"}
      >
        <header style={{ justifyContent: "flex-end" }}>
          <button aria-label="Закрыть карточку" onClick={attemptClose}>
            ×
          </button>
        </header>
        <form
          onChange={() => {
            dirty.current = true;
          }}
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
        >
          <fieldset
            disabled={uncertain || busy}
            style={{ border: 0, padding: 0, margin: 0, minWidth: 0 }}
          >
            <small className="task-project-path">
              {projectPath(projects, project || null)}
            </small>
            <label>
              Название
              <input
                autoFocus
                required
                maxLength={500}
                value={title}
                onChange={(e) => setTitle(e.target.value)}
              />
            </label>
            <div className="fields">
              <label>
                Проект
                <select
                  value={project}
                  onChange={(e) => setProject(e.target.value)}
                >
                  <option value="">Без проекта</option>
                  {projects.map((p) => (
                    <option key={p.id} value={p.id}>
                      {projectPath(projects, p.id)}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                Состояние
                <select
                  value={state}
                  onChange={(e) => setState(e.target.value)}
                  disabled={busy}
                >
                  <option value="not_started">Не начата</option>
                  <option value="active" disabled={initialState === "done"}>
                    В работе
                  </option>
                  <option value="paused" disabled={initialState === "done"}>
                    На паузе
                  </option>
                  <option value="background" disabled={initialState === "done"}>
                    Выполняется в фоне
                  </option>
                  <option value="waiting" disabled={initialState === "done"}>
                    Ожидание
                  </option>
                  <option value="done">Завершена</option>
                </select>
              </label>
            </div>
            {["waiting", "background"].includes(state) && (
              <div className="fields">
                <label>
                  Причина / следующий шаг
                  <input
                    required
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                  />
                </label>
                <label>
                  Когда проверить
                  <input
                    required
                    type="datetime-local"
                    value={checkAt}
                    onChange={(e) => setCheckAt(e.target.value)}
                  />
                </label>
              </div>
            )}
            <label>
              Описание
              <textarea
                value={description}
                maxLength={20000}
                onChange={(e) => setDescription(e.target.value)}
              />
            </label>
            <label>
              Заметка для возвращения
              <textarea
                value={resume}
                maxLength={5000}
                onChange={(e) => setResume(e.target.value)}
                placeholder="На чём остановились и что делать дальше"
              />
            </label>
            <h3>Подпункты</h3>
            {subtasks.map((s, i) => (
              <div className="subtask" key={s.id ?? s.clientRef}>
                <input
                  aria-label={`Готово: ${s.title}`}
                  type="checkbox"
                  checked={s.done}
                  onChange={(e) =>
                    setSubtasks(
                      subtasks.map((v, j) =>
                        j === i ? { ...v, done: e.target.checked } : v,
                      ),
                    )
                  }
                />
                <input
                  aria-label={`Подпункт ${i + 1}`}
                  required
                  value={s.title}
                  onChange={(e) =>
                    setSubtasks(
                      subtasks.map((v, j) =>
                        j === i ? { ...v, title: e.target.value } : v,
                      ),
                    )
                  }
                />
                <button
                  type="button"
                  aria-label={`Удалить подпункт ${i + 1}`}
                  onClick={() => {
                    dirty.current = true;
                    setSubtasks(subtasks.filter((_, j) => i !== j));
                  }}
                >
                  ×
                </button>
              </div>
            ))}
            <button
              type="button"
              onClick={() => {
                dirty.current = true;
                setSubtasks([
                  ...subtasks,
                  {
                    clientRef: crypto.randomUUID(),
                    title: "",
                    done: false,
                    position: subtasks.length,
                  },
                ]);
              }}
            >
              + Подпункт
            </button>
            <h3>Время и планирование</h3>
            <div className="fields">
              <label>
                Сложность
                <select
                  value={complexity}
                  onChange={(e) => setComplexity(e.target.value)}
                >
                  <option value="">Не определена</option>
                  <option value="easy">Лёгкая</option>
                  <option value="medium">Средняя</option>
                  <option value="hard">Сложная</option>
                </select>
              </label>
              <label>
                Контекст
                <select
                  value={context}
                  onChange={(e) => setContext(e.target.value)}
                >
                  <option value="anywhere">Где угодно</option>
                  <option value="computer">За компьютером</option>
                  <option value="away">Вне компьютера</option>
                </select>
              </label>
            </div>
            <div className="fields">
              <label>
                Оценка, мин
                <input
                  type="number"
                  min="1"
                  max="525600"
                  placeholder="Неизвестно"
                  value={estimate}
                  onChange={(e) => setEstimate(e.target.value)}
                />
              </label>
              <label>
                Осталось, мин
                <input
                  type="number"
                  min="0"
                  max="525600"
                  value={remaining}
                  onChange={(e) => setRemaining(e.target.value)}
                />
              </label>
            </div>
            <label className="check">
              <input
                type="checkbox"
                checked={multiDay}
                onChange={(e) => setMultiDay(e.target.checked)}
              />
              Многодневная
            </label>
<label className="check"><input type="checkbox" checked={splittable} onChange={e=>setSplittable(e.target.checked)}/> Разрешить деление на рабочие блоки</label>
            <SchedulingFields
              value={schedule}
              onChange={(v) => {
                dirty.current = true;
                setSchedule(v);
              }}
            />
            <section>
              <h3>Связанные задачи</h3>
              <p>
                Двусторонняя связь сохраняет завершённую задачу на диаграмме.
                Она не меняет сроки других задач.
              </p>
              {links.map((id) => (
                <div className="folder-row" key={id}>
                  <span>
                    {tasks.find((t) => t.id === id)
                      ? taskPath(projects, tasks.find((t) => t.id === id)!)
                      : id}
                  </span>
                  <button
                    type="button"
                    onClick={() => {
                      dirty.current = true;
                      setLinks(links.filter((v) => v !== id));
                    }}
                  >
                    Убрать связь
                  </button>
                </div>
              ))}
              <label>
                Добавить связь
                <select
                  value=""
                  onChange={(e) => {
                    dirty.current = true;
                    setLinks([...links, e.target.value]);
                  }}
                >
                  <option value="">Выберите задачу</option>
                  {tasks
                    .filter((t) => t.id !== task?.id && !links.includes(t.id))
                    .map((t) => (
                      <option key={t.id} value={t.id}>
                        {taskPath(projects, t)}
                      </option>
                    ))}
                </select>
              </label>
            </section>
            <p className="unavailable">
              Повторения и автоматическое распределение по дням ожидают
              согласования общих правил.
            </p>
            <h3>Приоритет и включение в план</h3>
            <div className="fields">
              {[
                ["Важность", importance, setImportance],
                ["Срочность", urgency, setUrgency],
              ].map(([label, value, set]) => (
                <label key={label as string}>
                  {label as string}
                  <select
                    value={value as string}
                    onChange={(e) =>
                      (set as (v: string) => void)(e.target.value)
                    }
                  >
                    <option value="unknown">Не определена</option>
                    <option value="yes">
                      {label === "Важность" ? "Важно" : "Срочно"}
                    </option>
                    <option value="no">
                      {label === "Важность" ? "Не важно" : "Не срочно"}
                    </option>
                  </select>
                </label>
              ))}
            </div>
            <small>
              Добавление в текущий план доступно в меню задачи «Сегодня».
            </small>
            <h3>Комментарии</h3>
            {task?.comments.map((c) => (
              <blockquote key={c.id}>
                {c.text}
                <small>{new Date(c.createdAt).toLocaleString("ru")}</small>
              </blockquote>
            ))}
            <label>
              Новый комментарий
              <textarea
                value={comment}
                maxLength={5000}
                onChange={(e) => setComment(e.target.value)}
              />
            </label>
          </fieldset>
          {conflict && resolveConflict && (
            <button
              type="button"
              onClick={async () => {
                try {
                  const actual = await resolveConflict();
                  setConflict(false);
                  setError(
                    "Актуальные данные загружены. Проверьте черновик перед повторным сохранением. Текущие сроки: " +
                      (actual?.scheduled
                        ? [
                            scheduleDraft(
                              actual.scheduled,
                              actual.deadline,
                              zone,
                            ).start,
                            scheduleDraft(
                              actual.scheduled,
                              actual.deadline,
                              zone,
                            ).end || "без окончания",
                          ].join(" — ")
                        : "без расписания"),
                  );
                } catch (e) {
                  setError((e as Error).message);
                }
              }}
            >
              Загрузить актуальную версию, оставить черновик
            </button>
          )}
          {uncertain && (
            <p>
              Ответ сервера не получен. Повторное сохранение отправит тот же
              запрос; поля временно заблокированы.
            </p>
          )}
          {error && (
            <p role="alert" className="error">
              {error}
            </p>
          )}
          {state === "done" &&
            initialState !== "done" &&
            subtasks.some((s) => !s.done) && (
              <fieldset className="unavailable">
                <legend>Есть незавершённые подпункты</legend>
                <label>
                  Как завершить задачу?
                  <select
                    required
                    value={decision}
                    onChange={(e) => setDecision(e.target.value)}
                  >
                    <option value="">Выберите действие</option>
                    <option value="complete_all">
                      Завершить все подпункты
                    </option>
                    <option value="keep_open">Закрыть без их выполнения</option>
                  </select>
                </label>
              </fieldset>
            )}
          {discard && (
            <div role="alert" className="notice">
              Есть несохранённые изменения.
              <button type="button" onClick={() => setDiscard(false)}>
                Продолжить редактирование
              </button>
              <button type="button" onClick={close}>
                Отбросить изменения
              </button>
            </div>
          )}
          <footer>
            <button type="button" onClick={attemptClose}>
              Отмена
            </button>
            <button className="primary" disabled={busy || conflict}>
              {busy ? "Сохранение…" : "Сохранить"}
            </button>
          </footer>
        </form>
      </section>
    </div>
  );
}
