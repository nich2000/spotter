import { it, expect } from "vitest";
import { projectScope } from "./projectScope";
it("includes each descendant once, handles malformed cycles and excludes unrelated roots", () => {
  expect([
    ...projectScope(
      [
        { id: "a", name: "A", parentId: "b" },
        { id: "b", name: "B", parentId: "a" },
        { id: "c", name: "C", parentId: "b" },
        { id: "d", name: "D", parentId: null },
      ],
      "a",
    ),
  ]).toEqual(["a", "b", "c"]);
});
