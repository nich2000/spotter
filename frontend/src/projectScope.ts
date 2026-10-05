import { Project } from "./api";
export function projectScope(projects: Project[], id: string): Set<string> {
  const result = new Set([id]);
  let changed = true;
  while (changed) {
    changed = false;
    for (const p of projects) {
      if (p.parentId && result.has(p.parentId) && !result.has(p.id)) {
        result.add(p.id);
        changed = true;
      }
    }
  }
  return result;
}
