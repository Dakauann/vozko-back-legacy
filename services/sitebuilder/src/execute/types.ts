import type { Project } from "../project";
import type { Run } from "./inline";
import type { Splice } from "./patcher";

export type OperationKind =
  | "write_file" | "edit_file" | "delete_file" | "rename_file"
  | "set_text" | "set_attr" | "set_classes" | "replace_image" | "move" | "duplicate" | "remove" | "insert_section"
  | "set_token" | "set_knob";

export const operationKinds: readonly OperationKind[] = [
  "write_file", "edit_file", "delete_file", "rename_file",
  "set_text", "set_attr", "set_classes", "replace_image", "move", "duplicate", "remove", "insert_section",
  "set_token", "set_knob",
];

export interface Placement {
  before?: string;
  after?: string;
  into?: string;
  page?: string;
}

export interface Operation {
  op: OperationKind;
  path?: string;
  to?: string;
  content?: string;
  old?: string;
  new?: string;
  replace_all?: boolean;
  node?: string;
  rev?: number;
  item?: number;
  text?: string;
  name?: string;
  value?: string | number;
  add?: string[];
  remove?: string[];
  breakpoint?: string;
  media_id?: string;
  alt?: string;
  before?: string;
  after?: string;
  into?: string;
  at?: Placement;
  html?: string;
  library_key?: string;
  ref?: string;
  source?: string;
  source_rev?: number;
}

export interface FileBase {
  rev: number;
  content: string;
}

export interface ExecuteRequest {
  project: Project;
  operations: Operation[];
  texts: Record<string, Run[]>;
  media: Record<string, string>;
  sections: Record<string, string>;
  jsx_sections?: Record<string, string>;
  bases: Record<string, FileBase[]>;
}

export interface ExtractRequest {
  project: Project;
  node: string;
  rev?: number;
  bases: Record<string, FileBase[]>;
}

export type ExtractResult = { source: string } | { failure: Omit<Failure, "index"> };

export interface StepPatch {
  op: OperationKind;
  index: number;
  focus?: string;
  files?: Record<string, string>;
  deleted?: string[];
  renamed?: Array<{ from: string; to: string }>;
}

export interface StepSplice {
  index: number;
  path: string;
  start: number;
  end: number;
  length: number;
}

export interface Failure {
  index: number;
  code: string;
  field?: string;
  detail?: string;
}

export interface Skip {
  index: number;
  after: number;
}

export type ExecuteResult =
  | { steps: StepPatch[]; splices: StepSplice[] }
  | { failures: Failure[]; skipped?: Skip[] };

export class OpFailure extends Error {
  constructor(readonly code: string, readonly field?: string, readonly detail?: string) {
    super(code);
  }
}

export interface FileChange {
  content: string;
  splices: Splice[];
  moved?: { from: number; to: number };
}

export interface OpChange {
  files: Record<string, FileChange>;
  deleted?: string[];
  renamed?: Array<{ from: string; to: string }>;
  focus?: string;
  created?: { file: string; offset: number };
}
