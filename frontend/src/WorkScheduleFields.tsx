export function WorkScheduleFields({
  settings: s,
  onChange,
}: {
  settings: Record<string, any>;
  onChange: (s: Record<string, any>) => void;
}) {
  const days = ["Вс", "Пн", "Вт", "Ср", "Чт", "Пт", "Сб"];
  const update = (key: string, k: string, value: any) =>
    onChange({ ...s, [key]: { ...s[key], [k]: value } });
  return (
    <fieldset>
      <legend>Расписание календаря</legend>
      <p>
        Резерв календаря — 20%. Рабочие блоки используют отдельные настройки
        Pomodoro.
      </p>
      {[
        ["pomodoroMinutes", "Рабочий интервал", 25],
        ["pomodoroBreakMinutes", "Короткий перерыв", 5],
        ["pomodoroLongBreakMinutes", "Перерыв после четырёх интервалов", 15],
      ].map(([key, label, value]) => (
        <label key={key}>
          {label}
          <input
            type="number"
            min="1"
            max="120"
            value={s[key] ?? value}
            onChange={(e) => onChange({ ...s, [key]: Number(e.target.value) })}
          />
        </label>
      ))}
      {[1, 2, 3, 4, 5, 6, 0].map((d) => {
        const k = String(d),
          w =
            s.workWeek && k in s.workWeek
              ? s.workWeek[k]
              : d === 0 || d === 6
                ? null
                : { start: s.workStart, end: s.workEnd };
        return (
          <div key={d} className="calendar-two">
            <label>
              <input
                type="checkbox"
                checked={!!w}
                onChange={(e) =>
                  update(
                    "workWeek",
                    k,
                    e.target.checked
                      ? { start: s.workStart, end: s.workEnd }
                      : null,
                  )
                }
              />
              {days[d]}
            </label>
            {w && (
              <div>
                <input
                  aria-label={days[d] + " начало"}
                  type="time"
                  value={w.start}
                  onChange={(e) =>
                    update("workWeek", k, { ...w, start: e.target.value })
                  }
                />
                <input
                  aria-label={days[d] + " конец"}
                  type="time"
                  value={w.end}
                  onChange={(e) =>
                    update("workWeek", k, { ...w, end: e.target.value })
                  }
                />
              </div>
            )}
          </div>
        );
      })}
      <details>
        <summary>Корректировки отдельных дней</summary>
        {Object.entries(s.workExceptions ?? {}).map(
          ([date, w]: [string, any]) => (
            <div key={date}>
              {date}
              <input
                aria-label={date + " рабочий день"}
                type="checkbox"
                checked={!!w}
                onChange={(e) =>
                  update(
                    "workExceptions",
                    date,
                    e.target.checked
                      ? { start: s.workStart, end: s.workEnd }
                      : null,
                  )
                }
              />
              {w && (
                <>
                  <input
                    aria-label={date + " начало"}
                    type="time"
                    value={w.start}
                    onChange={(e) =>
                      update("workExceptions", date, {
                        ...w,
                        start: e.target.value,
                      })
                    }
                  />
                  <input
                    aria-label={date + " конец"}
                    type="time"
                    value={w.end}
                    onChange={(e) =>
                      update("workExceptions", date, {
                        ...w,
                        end: e.target.value,
                      })
                    }
                  />
                </>
              )}
              <button
                type="button"
                onClick={() => {
                  const next = { ...s.workExceptions };
                  delete next[date];
                  onChange({ ...s, workExceptions: next });
                }}
              >
                Убрать корректировку
              </button>
            </div>
          ),
        )}
        <label>
          Добавить дату
          <input
            type="date"
            value=""
            onChange={(e) => {
              if (e.target.value)
                update("workExceptions", e.target.value, {
                  start: s.workStart,
                  end: s.workEnd,
                });
            }}
          />
        </label>
      </details>
    </fieldset>
  );
}
