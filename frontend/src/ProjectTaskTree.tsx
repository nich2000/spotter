import { useState, useEffect, type ReactNode } from "react";
import type { Project, Task } from "./api";
import { projectScope } from "./projectScope";
import { projectPath } from "./projectPaths";
export type TreeCardOptions = { inTree?: boolean; draggable?: boolean };
type Props = {
  projects: Project[];
  tasks: Task[];
  renderTask: (task: Task, depth: number) => ReactNode;
  rootId?: string;
  resetKey?: string;
  timeline?: boolean;
  onMove?: (taskId: string, projectId: string | null) => Promise<void>;
  onSelect?: (id: string) => void;
  selected?: string;
  projectActions?: (project: Project) => ReactNode;
};
export function ProjectTaskTree({
  projects,
  tasks,
  renderTask,
  rootId = "",
  resetKey = "",
  timeline = false,
  onMove,
  onSelect,
  selected,
  projectActions,
}: Props) {
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set()),
    [error, setError] = useState(""),
    [moving, setMoving] = useState(false);
  useEffect(() => setCollapsed(new Set()), [resetKey, rootId]);
  const visible = rootId
    ? projects.filter((p) => projectScope(projects, rootId).has(p.id))
    : projects;
  const visited = new Set<string>();
  const toggle = (id: string) =>
    setCollapsed((old) => {
      const n = new Set(old);
      n.has(id) ? n.delete(id) : n.add(id);
      return n;
    });
  const drop = async (e: React.DragEvent, id: string | null) => {
    if (!onMove) return;
    e.preventDefault();
    e.stopPropagation();
    const taskId = e.dataTransfer.getData("text/spotter-task"),
      t = tasks.find((t) => t.id === taskId);
    if (!t || t.projectId === id || moving) return;
    setMoving(true);
    setError("");
    try {
      await onMove(taskId, id);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setMoving(false);
    }
  };
  function group(p: Project | null, depth: number): ReactNode {
    const id = p?.id ?? "",
      name = p?.name ?? "Без проекта";
    if (p) {
      if (visited.has(id)) return null;
      visited.add(id);
    }
    const children = p ? visible.filter((c) => c.parentId === id) : [],
      scope = p ? projectScope(projects, id) : new Set<string>();
    const belongs = (t: Task) =>
      p
        ? t.projectId === id
        : !t.projectId || !projects.some((p) => p.id === t.projectId);
    const own = tasks.filter(belongs),
      all = p ? tasks.filter((t) => scope.has(t.projectId ?? "")) : own;
    const closed = collapsed.has(id),
      path = p ? projectPath(projects, id) : name;
    const nested = children.map((c) => group(c, depth + 1));
    return (
      <div key={id || "unassigned"} className="project-task-branch">
        <div
          className={
            (timeline ? "planning-row planning-group" : "") +
            " project-tree-group " +
            (selected === id ? "chosen" : "")
          }
          onDragOver={
            onMove
              ? (e) => {
                  e.preventDefault();
                  e.stopPropagation();
                }
              : undefined
          }
          onDrop={onMove ? (e) => void drop(e, p?.id ?? null) : undefined}
          data-project-id={id}
        >
          <div
            className={
              (timeline ? "planning-label " : "") + "project-tree-label"
            }
            style={{ paddingLeft: 12 + depth * 16 }}
          >
            <button
              className="tree-toggle"
              aria-label={"Развернуть или свернуть " + path}
              aria-expanded={!closed}
              onClick={() => toggle(id)}
            >
              {closed ? "▸" : "▾"}
            </button>
            {onSelect ? (
              <button
                className="tree-project-name"
                onClick={() => onSelect(id)}
                title={path}
              >
                {name}
              </button>
            ) : (
              <span className="tree-project-name" title={path}>
                {name}
              </span>
            )}
            {!timeline && p && projectActions?.(p)}
            {!timeline && (
              <small>
                {all.filter((t) => t.lifecycleState !== "done").length} открытых
                · {all.filter((t) => t.lifecycleState === "done").length}{" "}
                завершено
              </small>
            )}
          </div>
          {timeline && <div className="planning-lane" />}
        </div>
        {!closed && (
          <>
            <div className={timeline ? "" : "compact-list tree-tasks"}>
              {own.map((t) => (
                <div
                  key={t.id}
                  className="project-tree-task"
                  style={
                    timeline
                      ? undefined
                      : { paddingLeft: 12 + (depth + 1) * 16 }
                  }
                >
                  {renderTask(t, depth + 1)}
                </div>
              ))}
            </div>
            {nested}
          </>
        )}
      </div>
    );
  }
  const roots = visible.filter(
    (p) => !p.parentId || !visible.some((v) => v.id === p.parentId),
  );
  const nodes = roots.map((p) => group(p, 0));
  for (const p of visible) if (!visited.has(p.id)) nodes.push(group(p, 0));
  return (
    <div
      className={"shared-project-tree " + (timeline ? "timeline-tree" : "")}
      aria-busy={moving}
    >
      {error && <p role="alert">{error}</p>}
      {nodes}
      {!rootId && group(null, 0)}
      {!tasks.length && <p className="tree-empty">Нет задач</p>}
    </div>
  );
}
