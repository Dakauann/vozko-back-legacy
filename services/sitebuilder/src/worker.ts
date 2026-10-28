import { runJob, type JobKind } from "./jobs";

declare const self: Worker;

self.onmessage = async (event: MessageEvent<{ id: number; kind: JobKind; raw: string }>) => {
  const { id, kind, raw } = event.data;
  const reply = await runJob(kind, raw);
  self.postMessage({ id, ...reply });
};
