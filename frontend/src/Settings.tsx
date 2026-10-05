import { WorkScheduleFields } from "./WorkScheduleFields";
import { useEffect, useState } from "react";
import { post, request, Workspace } from "./api";
type Save = (name: string, payload: unknown) => Promise<void>;
export function Settings({
  workspace,
  save,
}: {
  workspace: Workspace;
  save: Save;
}) {
  const [settings, setSettings] = useState(workspace.settings);
  const [devices, setDevices] = useState<any[]>([]);
  const [consents, setConsents] = useState<any[]>([]);
  const [sources, setSources] = useState<any[]>([]);
  const [code, setCode] = useState("");
  const [message, setMessage] = useState("");
  async function refresh() {
    try {
      const [d, c, s] = await Promise.all([
        request("/devices"),
        request("/consents"),
        request("/sources"),
      ]);
      setDevices(d.items);
      setConsents(c.items);
      setSources(s.items);
    } catch (e) {
      setMessage((e as Error).message);
    }
  }
  useEffect(() => {
    void refresh();
  }, []);
  async function perform(fn: () => Promise<unknown>) {
    try {
      await fn();
      setMessage("Сохранено");
      await refresh();
    } catch (e) {
      setMessage((e as Error).message);
    }
  }
  return (
    <div className="settings-grid">
      <section className="panel">
        <h2>Рабочий ритм</h2>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void perform(() => save("settings.update", { patch: settings }));
          }}
        >
          {[
            ["zone", "Часовой пояс", "text"],
            ["workStart", "Начало дня", "time"],
            ["workEnd", "Конец дня", "time"],
            ["reservePercent", "Резерв, %", "number"],
            ["focusBlockMinutes", "Блок фокуса, мин", "number"],
            ["scheduleBreakMinutes", "Перерыв, мин", "number"],
            ["plannerSleepTargetHours", "Личная цель сна, ч", "number"],
          ].map(([key, label, type]) => (
            <label key={key}>
              {label}
              <input
                type={type}
                value={settings[key]}
                onChange={(e) =>
                  setSettings({
                    ...settings,
                    [key]:
                      type === "number"
                        ? Number(e.target.value)
                        : e.target.value,
                  })
                }
              />
            </label>
          ))}
          <WorkScheduleFields settings={settings} onChange={setSettings}/>
          <button className="primary">Сохранить настройки</button>
        </form>
      </section>
      <div>
        <section className="panel">
          <h2>Ваши данные</h2>
          <p>
            Дневник и здоровье сохраняются в локальном Docker. Эти категории не
            передаются модели.
          </p>
          {consents.map((c) => (
            <label className="check" key={c.category}>
              <input
                type="checkbox"
                checked={c.enabled}
                onChange={(e) =>
                  void perform(() =>
                    post("/consents", {
                      category: c.category,
                      enabled: e.target.checked,
                    }),
                  )
                }
              />
              {c.category === "diary"
                ? "Разрешить хранение дневника"
                : "Разрешить передачу здоровья с устройства"}
            </label>
          ))}
          <small>
            Отключение прекращает новые записи. Уже сохранённые данные не
            удаляются.
          </small>
        </section>
        <section className="panel">
          <h2>MacBook и источники</h2>
          <p>
            Helper запускается отдельно на Mac. Создайте одноразовый код и
            выполните <code>./run_helper.sh</code>.
          </p>
          <button
            onClick={() =>
              void perform(async () => {
                const data = await post("/device-enrollments", {});
                setCode(data.code);
              })
            }
          >
            Получить код сопряжения
          </button>
          {code && (
            <div className="pairing">
              <code>{code}</code>
              <small>Действует 5 минут. Показывается только владельцу.</small>
            </div>
          )}
          {devices.map((d) => (
            <div className="device" key={d.id}>
              <strong>{d.name}</strong>
              <small>
                {d.active
                  ? d.lastSeen
                    ? `Последняя доставка: ${new Date(d.lastSeen).toLocaleString("ru")}`
                    : "Ожидает первой доставки"
                  : "Доступ отозван"}
              </small>
              {d.active && (
                <button
                  onClick={() =>
                    void perform(() => post(`/devices/${d.id}/revoke`, {}))
                  }
                >
                  Отозвать доступ
                </button>
              )}
            </div>
          ))}
          {sources.map((s) => (
            <div className="device" key={s.deviceId + s.name}>
              <strong>{s.name}</strong>
              <span>{s.snapshot.ok ? "Получен снимок" : "Ошибка сбора"}</span>
              <small>{new Date(s.receivedAt).toLocaleString("ru")}</small>
            </div>
          ))}
          {!devices.length && <p>Устройства ещё не подключены.</p>}
        </section>
      </div>
      {message && (
        <p role="status" className="notice">
          {message}
        </p>
      )}
    </div>
  );
}
export function Wellbeing({
  date,
  workspace,
  save,
}: {
  date: string;
  workspace: Workspace;
  save: Save;
}) {
  const [day, setDay] = useState(date);
  const [record, setRecord] = useState<any>(workspace.days[date] ?? {});
  const [message, setMessage] = useState("");
  const [confirm, setConfirm] = useState(false);
  const [habitsConfirm, setHabitsConfirm] = useState(false);
  function changeDate(value: string) {
    setDay(value);
    setRecord(workspace.days[value] ?? {});
    setConfirm(false);
    setHabitsConfirm(false);
  }
  const current = record.wellbeing ?? {
    mood: "unknown",
    energy: "unknown",
    stress: "unknown",
  };
  return (
    <>
      <div className="toolbar">
        <label>
          Дата
          <input
            type="date"
            value={day}
            onChange={(e) => changeDate(e.target.value)}
          />
        </label>
        <span>
          {workspace.days[day] ? "Запись сохранена" : "За этот день нет записи"}
        </span>
      </div>
      <div className="wellbeing-layout">
        <section className="panel">
          <div className="rings">
            <div>
              <div>—</div>
            </div>
          </div>
          <h2>День по моим ориентирам</h2>
          <p>Сон и самочувствие · Активность · Вредные привычки</p>
          <small>
            Личные условия, не медицинская оценка. Без общего балла.
          </small>
          <p>
            Формулы колец ожидают выбора R7/R8. Отсутствие данных не означает
            ноль или хорошее самочувствие.
          </p>
          <p>Сон, шаги и другие измерения: нет подтверждённых данных.</p>
        </section>
        <form
          className="wellbeing-form"
          onSubmit={async (e) => {
            e.preventDefault();
            try {
              await save("wellbeing.save", {
                date: day,
                zone: workspace.settings.zone,
                note: record.note ?? "",
                wellbeing: { ...current, confirm },
                habits: {
                  ...(record.habits ?? {
                    cigarettes: null,
                    alcoholPortions: null,
                  }),
                  confirm: habitsConfirm,
                },
              });
              setMessage("Состояние сохранено");
            } catch (e) {
              setMessage((e as Error).message);
            }
          }}
        >
          <details className="panel">
            <summary>Сон и активность</summary>
            <fieldset disabled className="unavailable">
              <div className="fields">
                <label>
                  Продолжительность сна, ч
                  <input
                    type="number"
                    placeholder="Нет подтверждённых данных"
                  />
                </label>
                <label>
                  Шаги за день
                  <input
                    type="number"
                    placeholder="Нет подтверждённых данных"
                  />
                </label>
              </div>
              <small>Выбор и ручной ввод измерений пока недоступны.</small>
            </fieldset>
          </details>
          <details className="panel">
            <summary>Самочувствие · настроение, энергия, стресс</summary>
            <p>Неизвестные значения остаются неизвестными.</p>
            {[
              [
                "mood",
                "Настроение",
                ["unknown", "good", "normal", "bad"],
                ["Не знаю", "Хорошее", "Нормальное", "Плохое"],
              ],
              [
                "energy",
                "Энергия",
                ["unknown", "high", "medium", "low"],
                ["Не знаю", "Высокая", "Средняя", "Низкая"],
              ],
              [
                "stress",
                "Стресс",
                ["unknown", "low", "medium", "high"],
                ["Не знаю", "Низкий", "Средний", "Высокий"],
              ],
            ].map(([key, label, values, labels]) => (
              <label key={key as string}>
                {label as string}
                <select
                  value={current[key as string]}
                  onChange={(e) =>
                    setRecord({
                      ...record,
                      wellbeing: {
                        ...current,
                        [key as string]: e.target.value,
                      },
                    })
                  }
                >
                  {(values as string[]).map((v, i) => (
                    <option key={v} value={v}>
                      {(labels as string[])[i]}
                    </option>
                  ))}
                </select>
              </label>
            ))}
            <label>
              Заметка
              <textarea
                maxLength={20000}
                value={record.note ?? ""}
                onChange={(e) => setRecord({ ...record, note: e.target.value })}
              />
            </label>
            <label className="check">
              <input
                type="checkbox"
                checked={confirm}
                onChange={(e) => setConfirm(e.target.checked)}
              />
              Подтверждаю введённое состояние
            </label>
          </details>
          <details className="panel">
            <summary>Вредные привычки</summary>
            <div className="fields">
              {[
                ["cigarettes", "Сигареты за день, шт."],
                ["alcoholPortions", "Алкоголь за день, условных порций"],
              ].map(([key, label]) => (
                <label key={key}>
                  {label}
                  <input
                    type="number"
                    min="0"
                    step="1"
                    value={record.habits?.[key] ?? ""}
                    onChange={(e) => {
                      setHabitsConfirm(false);
                      setRecord({
                        ...record,
                        habits: {
                          cigarettes: null,
                          alcoholPortions: null,
                          ...record.habits,
                          [key]:
                            e.target.value === ""
                              ? null
                              : Number(e.target.value),
                        },
                      });
                    }}
                  />
                </label>
              ))}
            </div>
            <label className="check">
              <input
                type="checkbox"
                checked={habitsConfirm}
                onChange={(e) => setHabitsConfirm(e.target.checked)}
              />
              Подтверждаю значения вредных привычек за день
            </label>
            <small>
              Пустое значение — неизвестно. Ноль учитывается после
              подтверждения.
            </small>
          </details>
          <details className="panel">
            <summary>Физические измерения</summary>
            <fieldset disabled className="unavailable">
              <legend>Нет записей</legend>
              <p>
                Отдельные измерения с временем и единицами. Ввод пока
                недоступен.
              </p>
              <div className="fields">
                <label>
                  Показатель
                  <select>
                    <option>Пульс</option>
                    <option>Давление</option>
                    <option>Температура</option>
                    <option>Насыщение кислородом</option>
                    <option>Глюкоза</option>
                  </select>
                </label>
                <label>
                  Время измерения
                  <input type="datetime-local" />
                </label>
                <label>
                  Значение
                  <input type="number" />
                </label>
              </div>
              <button>Добавить измерение</button>
            </fieldset>
          </details>
          <button className="primary">Сохранить запись</button>
          {message && <p role="status">{message}</p>}
        </form>
      </div>
    </>
  );
}
