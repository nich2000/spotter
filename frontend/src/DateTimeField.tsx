/** Native calendar/clock picker; the domain draft keeps its dotted date format. */
export function DateTimeField({ value, onChange, withTime = false, required = false, label }: {
  label?: string;
  value: string;
  onChange: (value: string) => void;
  withTime?: boolean;
  required?: boolean;
}) {
  return <span className="date-time-field">
    <input
      aria-label={label}
      type={withTime ? "datetime-local" : "date"}
      step={withTime ? 1 : undefined}
      required={required}
      value={value.replaceAll(".", "-").replace(" ", "T")}
      onChange={e => {
        let next = e.target.value.slice(0, 19).replaceAll("-", ".").replace("T", " ");
        if (withTime && next.length === 16) next += ":00";
        onChange(next);
      }}
    />
    {value && <small aria-hidden="true">{value}</small>}
  </span>;
}
