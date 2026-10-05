import {
  projectPath as formatProjectPath,
  type Project,
  type Task,
} from "./api";
export function projectPath(projects: Project[], id: string | null): string {
  const project = projects.find((p) => p.id === id);
  return project ? formatProjectPath(project, projects) : "Без проекта";
}
export function taskPath(projects: Project[], task: Task): string {
  return projectPath(projects, task.projectId) + " / " + task.title;
}
