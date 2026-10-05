import { useEffect, useState, useRef } from "react";
import { APIError, localDate, post, type Task, type Workspace } from "./api";
import { addDays, displayDate, displayInstant } from "./calendar";
import { localInstant, scheduleDates, deadlineDate } from "./scheduling";
import { taskPath } from "./projectPaths";
import { freezeCommand, type CommandRequest } from "./command";
import {
  monthDays,
  shiftMonth,
  onDay,
  dueOn,
  dayEntries,
  layoutEntries,
  weekRibbons,
  type CalendarEntry,
} from "./calendarLayout";
import "./calendarView.css";
type Props = {
  workspace: Workspace;
  revision: number;
  open: (t: Task) => void;
  commit: (c: CommandRequest) => Promise<unknown>;
  refresh: () => Promise<unknown>;
};
export function CalendarView({
  workspace: w,
  revision,
  open,
  commit,
  refresh,
}: Props) {
  const requestVersion = useRef(0);
  const [selectedIds, setSelectedIds] = useState<string[]>([]),
    [until, setUntil] = useState("");
  const zone = w.settings.zone || "Europe/Moscow",
    today = localDate(zone);
  const [month, setMonth] = useState(today),
    [date, setDate] = useState(today),
    [taskId, setTask] = useState(""),
    [time, setTime] = useState("09:00"),
    [minutes, setMinutes] = useState(50),
    [pinned, setPinned] = useState(false),
    [choice, setChoice] = useState(""),
    [message, setMessage] = useState(""),
    [busy, setBusy] = useState(false),
    [pending, setPending] = useState<{
      blocks: CalendarEntry[];
      command: CommandRequest;
    } | null>(null),
    [uncertain, setUncertain] = useState(false),
    [expanded, setExpanded] = useState<string[]>([]);
  const tasks = w.tasks.filter(
      (t) => t.lifecycleState !== "done" || (t.relatedTaskIds?.length ?? 0) > 0,
    ),
    days = monthDays(month),
    sources = w.calendarSources ?? [];
  const entries: CalendarEntry[] = [
    ...(w.workBlocks ?? []),
    ...sources.flatMap((s) => s.events.filter((e) => !e.cancelled)),
    ...tasks.flatMap((t) =>
      t.scheduled && t.scheduled.kind !== "date_range"
        ? [
            {
              id: "task-" + t.id,
              title: taskPath(w.projects, t),
              taskId: t.id,
              kind: "task" as const,
              startAt: t.scheduled.startAt,
              endAt: t.scheduled.endAt ?? t.scheduled.startAt,
            },
          ]
        : [],
    ),
  ];
  const title = (e: CalendarEntry) =>
    e.kind === "break"
      ? "Перерыв"
      : e.taskId
        ? taskPath(w.projects, w.tasks.find((t) => t.id === e.taskId)!)
        : e.title;
  const selected = dayEntries(entries, date, zone),
    rows = layoutEntries(entries, date, zone);
  const weekSetting =
    w.settings.workWeek?.[new Date(date + "T00:00:00Z").getUTCDay()];
  const windowSetting = w.settings.workExceptions?.[date] ?? weekSetting;
  const hour = (s: string) => Number(s?.split(":")[0]) || 0;
  const startHour = Math.min(
      hour(windowSetting?.start ?? w.settings.workStart ?? "09:00"),
      ...rows.map((e) => Math.floor(e.start / 60)),
    ),
    endHour = Math.max(
      hour(windowSetting?.end ?? w.settings.workEnd ?? "18:00") + 1,
      ...rows.map((e) => Math.ceil(e.end / 60)),
    );
  const discrepancies = (w.workBlocks ?? []).filter((e) => {
    const t = w.tasks.find((t) => t.id === e.taskId);
    if (!t) return false;
    const a = displayInstant(e.startAt, t.scheduled?.zone ?? zone)
        .slice(0, 10)
        .replaceAll(".", "-"),
      b = displayInstant(
        new Date(Date.parse(e.endAt) - 1).toISOString(),
        t.scheduled?.zone ?? zone,
      )
        .slice(0, 10)
        .replaceAll(".", "-");
    if (t.scheduled) {
      const [start, end] = scheduleDates(t.scheduled);
      if (a < start || (end && b > end)) return true;
    }
    return !!t.deadline && b > deadlineDate(t.deadline);
  });
  const blockMinutes = new Map<string, number>();
  for (const e of rows)
    if (e.kind === "work" && e.taskId)
      blockMinutes.set(
        e.taskId,
        (blockMinutes.get(e.taskId) ?? 0) + e.end - e.start,
      );
  const activePlan = w.plans.find(
    (p) => p.date === date && p.status === "active",
  );
  for (const item of activePlan?.items ?? [])
    blockMinutes.set(
      item.taskId,
      Math.max(blockMinutes.get(item.taskId) ?? 0, item.allocatedMinutes ?? 0),
    );
  const plannedMinutes = [...blockMinutes.values()].reduce((a, b) => a + b, 0);
  const clear = () => {
    if (uncertain) return;
    requestVersion.current++;
    setPending(null);
    setMessage("");
  };
  const select = (d: string) => {
    if (uncertain) return;
    setDate(d);
    clear();
  };
  useEffect(() => {
    const timer = setInterval(() => void refresh(), 30000);
    return () => clearInterval(timer);
  }, [refresh]);
  async function propose(automatic = false) {
    const requestId = ++requestVersion.current;
    setBusy(true);
    setMessage("");
    setPending(null);
    try {
      const startAt = localInstant(
        date + " " + time + (time.length === 5 ? ":00" : ""),
        zone,
        choice,
      );
      const payload = { taskId, startAt, minutes, zone, pinned, automatic };
      const result = await post("/calendar/proposal", payload);
      if (requestId !== requestVersion.current) return;
      setPending({
        blocks: result.blocks,
        command: freezeCommand(
          "workblocks.create",
          payload,
          result.workspaceRevision,
        ),
      });
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function save(command: CommandRequest) {
    setBusy(true);
    setMessage("");
    try {
      await commit(command);
      setPending(null);
      setUncertain(false);
      setMessage("Сохранено. Фокус и таймер не запущены.");
    } catch (e) {
      setMessage((e as Error).message);
      if (e instanceof APIError && e.status < 500) {
        setPending(null);
        setUncertain(false);
        await refresh();
      } else {
        setUncertain(true);
        setPending((p) => p ?? { blocks: [], command });
      }
    } finally {
      setBusy(false);
    }
  }
  async function distribute() {
    const requestId = ++requestVersion.current;
    setBusy(true);
    setPending(null);
    setMessage("");
    try {
      const payload = {
        taskIds: selectedIds,
        fromDate: date,
        toDate: until || date,
        zone,
      };
      const result = await post("/calendar/proposal", payload);
      if (requestId !== requestVersion.current) return;
      setPending({
        blocks: result.blocks,
        command: freezeCommand(
          "workblocks.create",
          payload,
          result.workspaceRevision,
        ),
      });
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const timeLabel = (e: CalendarEntry) =>
    displayInstant(e.startAt, zone) + " — " + displayInstant(e.endAt, zone);
  return (
    <div className="calendar-view">
      <div className="calendar-tools">
        <span>{zone}</span>
        <div>
          <button
            aria-label="Предыдущий месяц"
            disabled={uncertain}
            onClick={() => {
              setMonth(shiftMonth(month, -1));
              clear();
            }}
          >
            ←
          </button>
          <h2>
            {new Date(month.slice(0, 7) + "-01T00:00:00Z").toLocaleDateString(
              "ru-RU",
              { month: "long", year: "numeric", timeZone: "UTC" },
            )}
          </h2>
          <button
            aria-label="Следующий месяц"
            disabled={uncertain}
            onClick={() => {
              setMonth(shiftMonth(month, 1));
              clear();
            }}
          >
            →
          </button>
          <button
            disabled={uncertain}
            onClick={() => {
              setMonth(today);
              select(today);
            }}
          >
            Сегодня
          </button>
        </div>
      </div>
      <p className="calendar-source-status">
        {sources.length
          ? sources
              .map(
                (s) =>
                  `${s.ok ? "Календарь обновлён" : "Источник недоступен"}: ${s.observedAt ? displayInstant(s.observedAt, zone) : "нет успешного обновления"}`,
              )
              .join(" · ")
          : "Внешний календарь не подключён. Доступность встреч неизвестна."}
      </p>
      {discrepancies.length > 0 && (
        <p role="alert">
          Есть блоки за пределами интервала задачи или дедлайна. Они сохранены:
          откройте соответствующий день и уберите блок либо исправьте срок в
          карточке.
        </p>
      )}
      {sources.some(
        (s) =>
          !s.ok ||
          !s.observedAt ||
          Date.now() - Date.parse(s.observedAt) > 86400000 ||
          !s.from ||
          s.from.slice(0, 10) > date ||
          !s.to ||
          s.to.slice(0, 10) <= date,
      ) && (
        <p role="alert">
          Нет свежего покрытия внешнего календаря на выбранный день. Отсутствие
          встреч не означает свободное время. Обновите helper.
        </p>
      )}
      <div className="calendar-month">
        <div className="calendar-dow">
          {["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"].map((d) => (
            <span key={d}>{d}</span>
          ))}
        </div>
        {Array.from({ length: days.length / 7 }, (_, index) => {
          const week = days.slice(index * 7, index * 7 + 7),
            ribbons = weekRibbons(tasks, week),
            showAll = expanded.includes(week[0]),
            shown = showAll ? ribbons : ribbons.filter((r) => r.lane < 2),
            lanes = shown.reduce((n, r) => Math.max(n, r.lane + 1), 0);
          return (
            <div
              className="calendar-week"
              key={week[0]}
              style={{ minHeight: 110 + Math.max(0, lanes - 2) * 26 }}
            >
              <div className="calendar-dates">
                {week.map((d) => {
                  const ev = dayEntries(entries, d, zone),
                    due = tasks.filter((t) => dueOn(t, d));
                  return (
                    <button
                      key={d}
                      className={
                        "calendar-date " +
                        (d === date ? "selected " : "") +
                        (d === today ? "today " : "") +
                        (d.slice(0, 7) !== month.slice(0, 7) ? "off" : "")
                      }
                      aria-pressed={d === date}
                      aria-label={`${displayDate(d)}, событий ${ev.length}, дедлайнов ${due.length}`}
                      onClick={() => select(d)}
                    >
                      <span className="num">{Number(d.slice(8))}</span>
                      <span className="daynotes">
                        {due.length ? "◆ " + due.length + " " : ""}
                        {ev.length ? "● " + ev.length : ""}
                      </span>
                    </button>
                  );
                })}
              </div>
              <div className="calendar-ribbons">
                {shown.map((r) => (
                  <button
                    key={r.task.id}
                    className={
                      "calendar-ribbon " + (r.lane % 2 ? "second" : "")
                    }
                    style={{
                      gridColumn: `${r.start + 1} / ${r.end + 2}`,
                      gridRow: r.lane + 1,
                    }}
                    title={taskPath(w.projects, r.task)}
                    onClick={() => {
                      select(week[r.start]);
                      setTask(r.task.id);
                    }}
                    onDoubleClick={() => open(r.task)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        e.preventDefault();
                        open(r.task);
                      }
                    }}
                  >
                    {taskPath(w.projects, r.task)}
                  </button>
                ))}
              </div>
              {ribbons.length > shown.length && (
                <button
                  className="calendar-more"
                  onClick={() => setExpanded([...expanded, week[0]])}
                >
                  Ещё {ribbons.length - shown.length}
                </button>
              )}
            </div>
          );
        })}
      </div>
      <div className="calendar-legend">
        <span>━ Интервал задачи: не занятость</span>
        <span>● Рабочие блоки</span>
        <span>▣ Внешние встречи · просмотр</span>
        <span>◆ Дедлайн</span>
      </div>
      <div className="calendar-lower">
        <section>
          <h3>
            {displayDate(date)} · {zone}
          </h3>
          <p>
            Назначено: {Math.round(plannedMinutes)} мин · объём плана и блоков
            одной задачи не суммируется дважды.
          </p>
          {tasks
            .filter(
              (t) =>
                (onDay(t, date) && t.scheduled?.kind === "date_range") ||
                dueOn(t, date),
            )
            .map((t) => (
              <div className="calendar-interval" key={t.id}>
                {dueOn(t, date) ? "◆ Дедлайн · " : ""}
                {taskPath(w.projects, t)}{" "}
                <button onClick={() => open(t)}>Открыть</button>
              </div>
            ))}
          {selected
            .filter((e) => e.allDay)
            .map((e) => (
              <div className="calendar-interval" key={e.id}>
                Весь день · {title(e)} · ▣ Только просмотр
              </div>
            ))}
          <div
            className="calendar-hours"
            style={{ height: Math.max(1, endHour - startHour) * 60 }}
          >
            {Array.from({ length: endHour - startHour }, (_, i) => (
              <div
                className="calendar-hour"
                key={i}
                style={{ top: i * 60 }}
                onDragOver={(e) => e.preventDefault()}
                onDrop={(e) => {
                  e.preventDefault();
                  const id = e.dataTransfer.getData("text/spotter-task");
                  if (w.tasks.some((t) => t.id === id)) {
                    setTask(id);
                    setTime(String(startHour + i).padStart(2, "0") + ":00");
                    clear();
                  }
                }}
              >
                <span>{String(startHour + i).padStart(2, "0")}:00</span>
                <button
                  aria-label={`Назначить на ${startHour + i}:00`}
                  disabled={uncertain}
                  onClick={() => {
                    setTime(String(startHour + i).padStart(2, "0") + ":00");
                    clear();
                  }}
                >
                  +
                </button>
              </div>
            ))}
            {rows.map((e) => (
              <div
                key={e.id}
                className={"calendar-event " + e.kind}
                style={{
                  top: e.start - startHour * 60,
                  height: Math.max(22, e.end - e.start),
                  left: `calc(54px + (100% - 56px) * ${e.lane / e.lanes})`,
                  width: `calc((100% - 56px) / ${e.lanes} - 3px)`,
                }}
                title={title(e) + " · " + timeLabel(e)}
              >
                {displayInstant(e.startAt, zone).slice(11, 16)} · {title(e)}
                {e.kind === "external" ? " · ▣ Только просмотр" : ""}
              </div>
            ))}
          </div>
          {!selected.length && <p>На этот день блоки не назначены</p>}
          {selected.map((e) => (
            <div className="calendar-listitem" key={e.id}>
              <strong>{title(e)}</strong>
              <div>
                {timeLabel(e)} ·{" "}
                {Math.round(
                  (Date.parse(e.endAt) - Date.parse(e.startAt)) / 60000,
                )}{" "}
                мин
              </div>
              {e.kind === "external" ? (
                <small>▣ {e.source} · Только просмотр</small>
              ) : e.kind === "task" ? (
                <button
                  onClick={() => open(w.tasks.find((t) => t.id === e.taskId)!)}
                >
                  Открыть
                </button>
              ) : (
                <>
                  <button
                    disabled={busy || uncertain}
                    onClick={() =>
                      void save(
                        freezeCommand(
                          "workblocks.delete",
                          { blockId: e.id },
                          revision,
                        ),
                      )
                    }
                  >
                    Убрать блок
                  </button>
                  <button
                    disabled={busy || uncertain}
                    onClick={() =>
                      void save(
                        freezeCommand(
                          "workblocks.pin",
                          { blockId: e.id, pinned: !e.pinned },
                          revision,
                        ),
                      )
                    }
                  >
                    {e.pinned ? "Открепить" : "Закрепить"}
                  </button>
                </>
              )}
            </div>
          ))}
        </section>
        <section className="calendar-assign">
          <h3>Назначить рабочий блок</h3>
          <fieldset disabled={busy || uncertain}>
            <label>
              Дата
              <input
                type="date"
                value={date}
                onChange={(e) => {
                  if (e.target.value) {
                    select(e.target.value);
                    setMonth(e.target.value);
                  }
                }}
              />
            </label>
            <label>
              Задача
              <select
                value={taskId}
                onChange={(e) => {
                  setTask(e.target.value);
                  clear();
                }}
              >
                <option value="">Выберите задачу</option>
                {w.tasks
                  .filter((t) => t.lifecycleState !== "done")
                  .map((t) => (
                    <option value={t.id} key={t.id}>
                      {taskPath(w.projects, t)}
                    </option>
                  ))}
              </select>
            </label>
            {taskId && (
              <div
                draggable
                onDragStart={(e) =>
                  e.dataTransfer.setData("text/spotter-task", taskId)
                }
                className="calendar-drag"
              >
                Перетащите выбранную задачу на час или задайте начало ниже
              </div>
            )}
            <div className="calendar-two">
              <label>
                Начало
                <input
                  type="time"
                  step="1"
                  value={time}
                  onChange={(e) => {
                    setTime(e.target.value);
                    clear();
                  }}
                />
              </label>
              <label>
                Работа, минут
                <input
                  type="number"
                  min="1"
                  max="1440"
                  value={minutes}
                  onChange={(e) => {
                    setMinutes(Number(e.target.value));
                    clear();
                  }}
                />
              </label>
            </div>
            <label>
              <input
                type="checkbox"
                checked={pinned}
                onChange={(e) => {
                  setPinned(e.target.checked);
                  clear();
                }}
              />{" "}
              Закрепить блоки
            </label>
            {message.includes("Неоднозначное") && (
              <label>
                Повторяющееся местное время
                <select
                  value={choice}
                  onChange={(e) => {
                    setChoice(e.target.value);
                    clear();
                  }}
                >
                  <option value="">Выберите вхождение</option>
                  <option value="earlier">Первое</option>
                  <option value="later">Второе</option>
                </select>
              </label>
            )}
            <p>
              {w.settings.pomodoroMinutes ?? 25} минут работы /{" "}
              {w.settings.pomodoroBreakMinutes ?? 5} минут перерыв; после
              четырёх — {w.settings.pomodoroLongBreakMinutes ?? 15} минут.
              Резерв 20%.
            </p>
            <button
              className="primary"
              disabled={!taskId}
              onClick={() => void propose()}
            >
              Предложить размещение
            </button>
            <button disabled={!taskId} onClick={() => void propose(true)}>
              Подобрать свободное время
            </button>
            <details>
              <summary>Распределить выбранные задачи по дням</summary>
              <label>
                До даты включительно
                <input
                  type="date"
                  value={until || date}
                  min={date}
                  max={addDays(date, 30)}
                  onChange={(e) => {
                    setUntil(e.target.value);
                    clear();
                  }}
                />
              </label>
              {w.tasks
                .filter((t) => t.lifecycleState !== "done")
                .map((t) => (
                  <label key={t.id}>
                    <input
                      type="checkbox"
                      checked={selectedIds.includes(t.id)}
                      onChange={(e) => {
                        setSelectedIds(
                          e.target.checked
                            ? [...selectedIds, t.id]
                            : selectedIds.filter((id) => id !== t.id),
                        );
                        clear();
                      }}
                    />
                    {taskPath(w.projects, t)}
                  </label>
                ))}
              <button
                disabled={!selectedIds.length}
                onClick={() => void distribute()}
              >
                Предложить распределение
              </button>
            </details>
          </fieldset>
          {pending && (
            <div className="calendar-preview">
              <h4>Предложение</h4>
              {pending.blocks.map((e, i) => (
                <p key={i}>
                  {e.kind === "break" ? "Перерыв" : title(e)} · {timeLabel(e)}
                </p>
              ))}
              <button
                disabled={busy}
                onClick={() => void save(pending.command)}
              >
                {uncertain ? "Повторить сохранение" : "Подтвердить"}
              </button>
              {!uncertain && (
                <button disabled={busy} onClick={() => setPending(null)}>
                  Отмена
                </button>
              )}
            </div>
          )}
          <p role="status" aria-live="polite">
            {message}
          </p>
          <p>Блоки не сохраняют план дня и не считаются выполненной работой.</p>
        </section>
      </div>
    </div>
  );
}
