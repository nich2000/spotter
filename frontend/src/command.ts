import { post } from "./api";
export type CommandRequest = {
  operationId: string;
  expectedRevision: number;
  command: string;
  payload: unknown;
};
// A retry sends exactly the same immutable envelope, including its original revision.
export function freezeCommand(
  command: string,
  payload: unknown,
  expectedRevision: number,
): CommandRequest {
  return JSON.parse(
    JSON.stringify({
      operationId: crypto.randomUUID(),
      expectedRevision,
      command,
      payload,
    }),
  );
}
export async function sendCommand(body: CommandRequest) {
  return post("/workspace/commands", body);
}
