import type { Scheduled, Deadline } from "./scheduling";
export class APIError extends Error {
  constructor(
    public code: string,
    message: string,
    public status: number,
  ) {
    super(message);
  }
}
export async function request<T = any>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const response = await fetch("/api/v2" + path, {
    ...options,
    credentials: "same-origin",
    cache: "no-store",
    headers: {
      "Content-Type": "application/json",
      "X-Spotter-Request": "workspace",
      ...options.headers,
    },
  });
  const value = await response.json();
  if (!response.ok)
    throw new APIError(
      value.error?.code ?? "HTTP_ERROR",
      value.error?.message ?? "Не удалось выполнить запрос",
      response.status,
    );
  return value;
}
export const post = <T = any>(path: string, value: unknown) =>
  request<T>(path, { method: "POST", body: JSON.stringify(value) });
export type Task = {
  scheduled?: Scheduled | null;
  deadline?: Deadline | null;
  relatedTaskIds?: string[];
  id: string;
  definitionId: string;
  title: string;
  description: string;
  resume: string;
  projectId: string | null;
  context: string;
  importance: string;
  urgency: string;
  reviewRequired: boolean;
  estimateMinutes: number | null;
  remainingMinutes: number | null;
  complexity: string | null;
  multiDay: boolean;
  splittable?: boolean;
  lifecycleState: string;
  workMode: string | null;
  completionEventId: string | null;
  subtasks: Array<{
    id: string;
    title: string;
    done: boolean;
    position: number;
  }>;
  comments: Array<{ id: string; text: string; createdAt: string }>;
};
export type Project = { id: string; name: string; parentId: string | null };
export type Plan = {
  id: string;
  date: string;
  status: string;
  items: Array<{
    taskId: string;
    allocatedMinutes: number | null;
    position: number;
  }>;
  mainOccurrenceId: string | null;
  frogOccurrenceId: string | null;
};
export type Workspace = {
 workBlocks?: import("./calendarLayout").CalendarEntry[];
 calendarSources?: CalendarSource[];
  candidates?: Array<{
    id: string;
    proposedTitle: string;
    sourceId: string;
    candidateVersion: number;
    sourceSnapshotId: string;
    state: string;
  }>;
  tasks: Task[];
  projects: Project[];
  plans: Plan[];
  settings: Record<string, any>;
  days: Record<string, any>;
  migration: Record<string, any>;
};
export function quadrant(task: Task): string {
  if (
    task.reviewRequired ||
    task.importance === "unknown" ||
    task.urgency === "unknown"
  )
    return "unreviewed";
  return task.importance === "yes"
    ? task.urgency === "yes"
      ? "q1"
      : "q2"
    : task.urgency === "yes"
      ? "q3"
      : "q4";
}
export function projectPath(project: Project, projects: Project[]): string {
  const result = [project.name];
  const seen = new Set([project.id]);
  let id = project.parentId;
  while (id) {
    const parent = projects.find((p) => p.id === id);
    if (!parent || seen.has(id)) break;
    seen.add(id);
    result.unshift(parent.name);
    id = parent.parentId;
  }
  return result.join(" / ");
}
export function localDate(zone = "Europe/Moscow"): string {
  return new Intl.DateTimeFormat("sv-SE", {
    timeZone: zone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(new Date());
}
export type CalendarSource = {id:string;ok:boolean;observedAt?:string;from?:string;to?:string;error?:string;events: (import('./calendarLayout').CalendarEntry & {availability?:string;cancelled?:boolean})[]};
