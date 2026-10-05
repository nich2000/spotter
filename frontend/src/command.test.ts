import { it, expect, vi } from "vitest";
import { freezeCommand, sendCommand } from "./command";
it("retains the same operation and revision after ambiguous network failure", async () => {
  const payload = { taskId: "t", patch: { title: "original" } };
  const body = freezeCommand("task.save", payload, 7);
  payload.patch.title = "changed";
  const fetch = vi
    .fn()
    .mockRejectedValueOnce(new TypeError("network"))
    .mockResolvedValue({
      ok: true,
      json: async () => ({ committedRevision: 8 }),
    });
  vi.stubGlobal("fetch", fetch);
  await expect(sendCommand(body)).rejects.toThrow("network");
  await sendCommand(body);
  expect(fetch.mock.calls[0][1].body).toBe(fetch.mock.calls[1][1].body);
  expect(JSON.parse(fetch.mock.calls[1][1].body)).toMatchObject({
    expectedRevision: 7,
    payload: { patch: { title: "original" } },
  });
});
