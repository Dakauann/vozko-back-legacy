import type { JobKind, JobReply } from "./jobs";

export class PoolFull extends Error {}
export class JobTimeout extends Error {}

interface Job {
  id: number;
  kind: JobKind;
  raw: string;
  resolve: (reply: JobReply) => void;
  reject: (error: Error) => void;
}

interface Slot {
  worker: Worker;
  job: Job | null;
  timer: ReturnType<typeof setTimeout> | null;
}

export interface PoolOptions {
  size: number;
  queueLimit: number;
  timeouts: Record<JobKind, number>;
  spawn?: () => Worker;
}

export class Pool {
  private readonly slots: Slot[] = [];
  private readonly queue: Job[] = [];
  private nextId = 1;
  private closed = false;

  constructor(private readonly options: PoolOptions) {
    for (let i = 0; i < options.size; i++) this.slots.push(this.slot());
  }

  get queued(): number {
    return this.queue.length;
  }

  get busy(): number {
    return this.slots.filter((slot) => slot.job !== null).length;
  }

  run(kind: JobKind, raw: string): Promise<JobReply> {
    if (this.closed) return Promise.reject(new PoolFull("the pool is closed"));
    const free = this.slots.find((slot) => slot.job === null);
    if (!free && this.queue.length >= this.options.queueLimit) return Promise.reject(new PoolFull("the builder queue is full"));
    return new Promise<JobReply>((resolve, reject) => {
      const job: Job = { id: this.nextId++, kind, raw, resolve, reject };
      if (free) this.dispatch(free, job);
      else this.queue.push(job);
    });
  }

  close() {
    this.closed = true;
    for (const job of this.queue.splice(0)) job.reject(new PoolFull("the pool is closed"));
    for (const slot of this.slots) {
      if (slot.timer) clearTimeout(slot.timer);
      slot.job?.reject(new PoolFull("the pool is closed"));
      slot.worker.terminate();
    }
  }

  private spawn(): Worker {
    return this.options.spawn ? this.options.spawn() : new Worker(new URL("./worker.ts", import.meta.url).href);
  }

  private slot(): Slot {
    const slot: Slot = { worker: this.spawn(), job: null, timer: null };
    this.listen(slot);
    return slot;
  }

  private listen(slot: Slot) {
    slot.worker.onmessage = (event: MessageEvent<{ id: number } & JobReply>) => {
      const job = slot.job;
      if (!job || event.data.id !== job.id) return;
      this.finish(slot);
      job.resolve({ status: event.data.status, body: event.data.body });
    };
    slot.worker.onerror = (event: ErrorEvent) => {
      const job = slot.job;
      this.replace(slot);
      job?.reject(new Error(event.message || "the builder worker crashed"));
    };
  }

  private dispatch(slot: Slot, job: Job) {
    slot.job = job;
    slot.timer = setTimeout(() => {
      this.replace(slot);
      job.reject(new JobTimeout(`the ${job.kind} job took too long`));
    }, this.options.timeouts[job.kind]);
    slot.worker.postMessage({ id: job.id, kind: job.kind, raw: job.raw });
  }

  private finish(slot: Slot) {
    if (slot.timer) clearTimeout(slot.timer);
    slot.timer = null;
    slot.job = null;
    this.next(slot);
  }

  private replace(slot: Slot) {
    if (slot.timer) clearTimeout(slot.timer);
    slot.timer = null;
    slot.job = null;
    slot.worker.terminate();
    if (this.closed) return;
    slot.worker = this.spawn();
    this.listen(slot);
    this.next(slot);
  }

  private next(slot: Slot) {
    const job = this.queue.shift();
    if (job) this.dispatch(slot, job);
  }
}
