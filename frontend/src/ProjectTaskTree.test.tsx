import { it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ProjectTaskTree } from "./ProjectTaskTree";
import { projectPath, taskPath } from "./projectPaths";
import type { Task, Project } from "./api";
const projects: Project[] = [
  { id: "a", name: "A", parentId: null },
  { id: "b", name: "Same", parentId: "a" },
  { id: "c", name: "C", parentId: null },
  { id: "d", name: "Same", parentId: "c" },
];
const tasks = [
  { id: "t", title: "Task", projectId: "b", lifecycleState: "not_started" },
  { id: "u", title: "Other", projectId: null, lifecycleState: "done" },
] as Task[];
const row = (t: Task) => <span>{t.title}</span>;
it("qualifies identical names, preserves unknown and cycle safety", () => {
  expect(projectPath(projects, "b")).toBe("A / Same");
  expect(taskPath(projects, tasks[0])).toBe("A / Same / Task");
  expect(taskPath(projects, { ...tasks[0], projectId: "d" })).toBe(
    "C / Same / Task",
  );
  expect(projectPath(projects, "missing")).toBe("Без проекта");
  expect(projectPath(projects, null)).toBe("Без проекта");
  expect(
    projectPath(
      [
        { id: "a", name: "A", parentId: "b" },
        { id: "b", name: "B", parentId: "a" },
      ],
      "a",
    ),
  ).toBe("B / A");
});
it("collapses complete nested branches without moving tasks in a read only tree", () => {
  const { container } = render(
    <ProjectTaskTree projects={projects} tasks={tasks} renderTask={row} />,
  );
  expect(screen.getByText("Task")).toBeInTheDocument();
  fireEvent.click(screen.getByLabelText("Развернуть или свернуть A"));
  expect(screen.queryByText("Task")).not.toBeInTheDocument();
  expect(
    screen.queryByLabelText("Развернуть или свернуть A / Same"),
  ).not.toBeInTheDocument();
  expect(screen.getByText("Other")).toBeInTheDocument();
  fireEvent.click(screen.getByLabelText("Развернуть или свернуть A"));
  expect(screen.getByText("Task")).toBeInTheDocument();
  const e = new Event("drop", { bubbles: true, cancelable: true });
  container.querySelector('[data-project-id="c"]')!.dispatchEvent(e);
  expect(e.defaultPrevented).toBe(false);
});
it("moves by stable IDs, ignores same project/foreign drags, and keeps source on failure", async () => {
  const move = vi
      .fn()
      .mockRejectedValueOnce(new Error("Conflict"))
      .mockResolvedValue(undefined),
    select = vi.fn();
  const { container } = render(
    <ProjectTaskTree
      projects={projects}
      tasks={tasks}
      renderTask={row}
      onMove={move}
      onSelect={select}
    />,
  );
  const target = container.querySelector('[data-project-id="c"]')!;
  fireEvent.dragOver(target);
  fireEvent.drop(container.querySelector('[data-project-id="b"]')!, {
    dataTransfer: { getData: () => "t" },
  });
  fireEvent.drop(target, { dataTransfer: { getData: () => "missing" } });
  expect(move).not.toHaveBeenCalled();
  fireEvent.drop(target, { dataTransfer: { getData: () => "t" } });
  expect(await screen.findByRole("alert")).toHaveTextContent("Conflict");
  expect(move).toHaveBeenCalledWith("t", "c");
  expect(screen.getByText("Task")).toBeInTheDocument();
  fireEvent.drop(container.querySelector('[data-project-id=""]')!, {
    dataTransfer: { getData: () => "t" },
  });
  await waitFor(() => expect(move).toHaveBeenLastCalledWith("t", null));
  fireEvent.click(
    screen.getByRole("button", { name: "C" }),
  );
  expect(select).toHaveBeenCalledWith("c");
});
it("filters descendants, resets disclosure for navigation, and renders malformed trees once", () => {
  const { rerender } = render(
    <ProjectTaskTree
      projects={projects}
      tasks={tasks.slice(0, 1)}
      renderTask={row}
      rootId="a"
      resetKey="1"
      timeline
    />,
  );
  expect(screen.queryByText("C")).not.toBeInTheDocument();
  fireEvent.click(screen.getByLabelText("Развернуть или свернуть A"));
  expect(screen.queryByText("Task")).not.toBeInTheDocument();
  rerender(
    <ProjectTaskTree
      projects={projects}
      tasks={tasks.slice(0, 1)}
      renderTask={row}
      rootId="a"
      resetKey="2"
      timeline
    />,
  );
  expect(screen.getByText("Task")).toBeInTheDocument();
  rerender(
    <ProjectTaskTree
      projects={[
        { id: "a", name: "A", parentId: "b" },
        { id: "b", name: "B", parentId: "a" },
      ]}
      tasks={[]}
      renderTask={row}
    />,
  );
  expect(screen.getByText("A")).toBeInTheDocument();
  expect(screen.getByText("B")).toBeInTheDocument();
  expect(screen.getByText("Нет задач")).toBeInTheDocument();
});
