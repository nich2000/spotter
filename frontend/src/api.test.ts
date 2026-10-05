import { describe, it, expect, vi } from "vitest";
import {
  request,
  post,
  APIError,
  quadrant,
  projectPath,
  localDate,
  Task,
} from "./api";
describe("API", () => {
  it("sends session and csrf protection and returns errors", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ value: 1 }) })
      .mockResolvedValueOnce({
        ok: false,
        status: 409,
        json: async () => ({
          error: { code: "VERSION_CONFLICT", message: "Draft retained" },
        }),
      })
      .mockResolvedValueOnce({
        ok: false,
        status: 500,
        json: async () => ({}),
      });
    vi.stubGlobal("fetch", fetch);
    expect(await post("/workspace/commands", { operationId: "one" })).toEqual({
      value: 1,
    });
    expect(fetch.mock.calls[0][1]).toMatchObject({
      credentials: "same-origin",
      cache: "no-store",
      headers: { "X-Spotter-Request": "workspace" },
      body: '{"operationId":"one"}',
    });
    await expect(request("/x")).rejects.toMatchObject({
      code: "VERSION_CONFLICT",
    });
    await expect(request("/x")).rejects.toBeInstanceOf(APIError);
  });
  it("keeps unknown priorities outside matrix", () => {
    for (const [a, b, want] of [
      ["yes", "yes", "q1"],
      ["yes", "no", "q2"],
      ["no", "yes", "q3"],
      ["no", "no", "q4"],
      ["unknown", "yes", "unreviewed"],
      ["yes", "unknown", "unreviewed"],
    ]) {
      expect(
        quadrant({ importance: a, urgency: b, reviewRequired: false } as Task),
      ).toBe(want);
    }
    expect(quadrant({ reviewRequired: true } as Task)).toBe("unreviewed");
  });
  it("builds paths and terminates cycles", () => {
    const a = { id: "a", name: "A", parentId: null },
      b = { id: "b", name: "B", parentId: "a" };
    expect(projectPath(b, [a, b])).toBe("A / B");
    expect(projectPath(b, [b])).toBe("B");
    expect(projectPath(b, [{ ...a, parentId: "b" }, b])).toBe("A / B");
    expect(localDate("UTC")).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });
});
