import { describe, expect, test } from "bun:test";
import { readFileSync, writeFileSync } from "node:fs";
import { execute } from "../src/execute/executor";
import type { Run } from "../src/execute/inline";
import type { ExecuteResult, Operation, StepPatch } from "../src/execute/types";
import type { Project, ProjectFile } from "../src/project";

const contractPath = new URL("../../../domain/site/testdata/edit_contract.json", import.meta.url);

interface Execution {
  texts?: Record<string, Run[]>;
  media?: Record<string, string>;
  sections?: Record<string, string>;
  steps?: StepPatch[];
  final?: Record<string, ProjectFile>;
  failures?: Array<{ index: number; code: string; field?: string; detail?: string }>;
  skipped?: Array<{ index: number; after: number }>;
}

interface Batch {
  name: string;
  base: string;
  accepted?: Operation[];
  executed?: Execution;
}

interface Replay {
  name: string;
  base: string;
  steps: StepPatch[];
  final?: Record<string, ProjectFile>;
  refusal?: string;
}

interface Contract {
  bases: Record<string, Project>;
  batches: Batch[];
  replays: Replay[];
}

const inputs: Record<string, Pick<Execution, "texts" | "media" | "sections">> = {
  "visual edits on a page": {
    texts: { "0": [{ text: "Olá " }, { text: "mundo", bold: true }] },
    media: { med_01HX: "https://cdn.vozko.test/media/med_01HX.webp" },
  },
  "code edits follow the file set": { texts: { "4": [{ text: "Quem somos" }] } },
  "refs chain duplicate, move and insert": { sections: { faq: '<section class="py-16">\n  <h2>Perguntas frequentes</h2>\n</section>' } },
  "react component text inside a mapped list": { texts: { "0": [{ text: "Lentes de contato" }] } },
};

const stepKeys = new Set(["op", "index", "focus", "files", "deleted", "renamed"]);

function applySteps(base: Project, steps: StepPatch[]): Record<string, ProjectFile> {
  const files: Record<string, ProjectFile> = structuredClone(base.files);
  for (const step of steps) {
    if (Object.keys(step).some((key) => !stepKeys.has(key))) throw new Error("steps_invalid");
    const bumped = new Set<string>();
    const bump = (path: string, rev: number) => {
      let next = rev;
      if (!bumped.has(path)) {
        next = rev + 1;
        bumped.add(path);
      }
      files[path] = { content: files[path].content, rev: next };
    };
    for (const rename of step.renamed ?? []) {
      const from = files[rename.from];
      if (!from || files[rename.to]) throw new Error("steps_invalid");
      delete files[rename.from];
      files[rename.to] = from;
      bump(rename.to, from.rev);
    }
    for (const path of step.deleted ?? []) {
      if (!files[path] || step.files?.[path] !== undefined) throw new Error("steps_invalid");
      delete files[path];
    }
    for (const path of Object.keys(step.files ?? {}).sort()) {
      const content = step.files![path];
      const current = files[path];
      if (current && current.content === content) continue;
      files[path] = { content, rev: current?.rev ?? 0 };
      if (!bumped.has(path)) bump(path, current?.rev ?? 0);
    }
  }
  return Object.fromEntries(Object.keys(files).sort().map((path) => [path, files[path]]));
}

function executionOf(batch: Batch, project: Project, result: ExecuteResult, given: Pick<Execution, "texts" | "media" | "sections">): Execution {
  const out: Execution = { ...given };
  if ("steps" in result) {
    out.steps = result.steps;
    out.final = applySteps(project, result.steps);
  } else {
    out.failures = result.failures;
    if (result.skipped) out.skipped = result.skipped;
  }
  return out;
}

async function run(contract: Contract, batch: Batch, given: Pick<Execution, "texts" | "media" | "sections">): Promise<Execution> {
  const project = contract.bases[batch.base];
  const result = await execute({
    project,
    operations: batch.accepted!,
    texts: given.texts ?? {},
    media: given.media ?? {},
    sections: given.sections ?? {},
    bases: {},
  });
  return executionOf(batch, project, result, given);
}

const contract: Contract = JSON.parse(readFileSync(contractPath, "utf8"));

if (process.env.UPDATE_CONTRACT === "1") {
  for (const batch of contract.batches) {
    if (!batch.accepted) continue;
    const given = inputs[batch.name] ?? {};
    batch.executed = await run(contract, batch, given);
  }
  writeFileSync(contractPath, JSON.stringify(contract, null, 2) + "\n");
}

describe("edit contract", () => {
  const accepted = contract.batches.filter((batch) => batch.accepted);

  test("every accepted batch has a recorded builder execution", () => {
    for (const batch of accepted) expect(batch.executed, batch.name).toBeDefined();
  });

  for (const batch of accepted) {
    test(`executes ${batch.name}`, async () => {
      const recorded = batch.executed!;
      const given = { texts: recorded.texts, media: recorded.media, sections: recorded.sections };
      const fresh = await run(contract, batch, Object.fromEntries(Object.entries(given).filter(([, v]) => v !== undefined)));
      expect(fresh).toEqual(recorded);
    });
  }

  for (const replay of contract.replays) {
    test(`replays ${replay.name}`, () => {
      const base = contract.bases[replay.base];
      if (replay.refusal) {
        expect(() => applySteps(base, replay.steps)).toThrow(replay.refusal);
        return;
      }
      expect(applySteps(base, replay.steps)).toEqual(replay.final!);
    });
  }
});
