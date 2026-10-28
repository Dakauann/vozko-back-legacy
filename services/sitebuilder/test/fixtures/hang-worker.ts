declare const self: Worker;

self.onmessage = (event: MessageEvent<{ id: number; raw: string }>) => {
  if (event.data.raw === "hang") {
    for (;;) {}
  }
  self.postMessage({ id: event.data.id, status: 200, body: event.data.raw });
};
