import { render, screen, fireEvent } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { DateTimeField } from "./DateTimeField";
it("uses a calendar for dates and preserves clearing", () => {
 const change = vi.fn();
 render(<label>Дата<DateTimeField label="Дата" value="2026.10.05" onChange={change} required /></label>);
 const input = screen.getByLabelText("Дата");
 expect(input).toHaveAttribute("type", "date");
 expect(input).toHaveValue("2026-10-05");
 fireEvent.change(input, {target:{value:"2026-10-07"}});
 expect(change).toHaveBeenLastCalledWith("2026.10.07");
 fireEvent.change(input, {target:{value:""}});
 expect(change).toHaveBeenLastCalledWith("");
});
it("uses a calendar and clock with seconds without changing timezone", () => {
 const change = vi.fn();
 render(<label>Срок<DateTimeField label="Срок" value="2026.10.05 13:14:15" onChange={change} withTime /></label>);
 const input = screen.getByLabelText("Срок");
 expect(input).toHaveAttribute("type", "datetime-local");
 expect(input).toHaveAttribute("step", "1");
 fireEvent.change(input, {target:{value:"2026-10-06T12:30"}});
 expect(change).toHaveBeenLastCalledWith("2026.10.06 12:30:00");
 fireEvent.change(input, {target:{value:"2026-10-06T12:30:45"}});
 expect(change).toHaveBeenLastCalledWith("2026.10.06 12:30:45");
});
