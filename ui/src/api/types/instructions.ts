export interface InstructionsWarning {
  code: string;
  params: Record<string, string>;
  message: string;
}

// Instruction files (CLAUDE.md, AGENTS.md, ...)
export interface InstructionsAssignment {
  name: string; // shared instruction file (a single-file extra)
  mode: string; // import | symlink | copy
  status: string; // synced | drift | modified | not synced | no source
  reason?: 'folder_link' | 'directory'; // why it is not synced: a folder link (Windows junction) the tool cannot read, or a real folder in the way
}

export interface InstructionsEntry {
  path: string;
  kind: 'main' | 'fallback' | 'rules' | 'unread';
  exists: boolean;
  read: boolean;
  count?: number;
}

export interface TargetInstructions {
  target: string;
  project: boolean;
  supported: boolean;
  custom: boolean; // the file is one the user set for this target
  setup?: TargetInstructionsSetup; // that setting as written
  path?: string;
  exists: boolean;
  content: string;
  size: number;
  link_to?: string;
  link_shared?: string; // the shared instruction file link_to points to
  import: boolean;
  max_chars?: number;
  read_order: InstructionsEntry[];
  import_lines: number[];
  shared: InstructionsAssignment[];
  convert: ConvertMethod[];
  convert_blocked?: Partial<Record<ConvertMethod, string>>;
  rider_of?: string; // not a target: reads this target's skills
  riders: InstructionsRider[]; // tools reading this target's skills from their own file
  read_by: string[]; // other tools that read this very file
  default_path?: string; // the built-in file, used when no location is set
}

/** Another file a target's tool reads, shown as its own tab on the target page. */
export interface TargetFile {
  path: string; // relative to the list's root, with /
  abs: string;
  builtin: boolean; // skillshare knows the tool reads it; the user can't remove it
  exists: boolean;
  size: number;
  link_to?: string;
  link_shared?: string; // the extra the file links to
}

export interface TargetFileList {
  target: string;
  project: boolean;
  root: string; // where added files may live; empty when the target can't add any
  files: TargetFile[];
}

/** Why adding a target file was refused (`reason` of a target_file_invalid_path error). */
export type TargetFileRefusal = 'outside' | 'absolute' | 'empty' | 'is_dir' | 'listed' | 'no_root';

/** A tool that reads a target's skills but keeps its own instruction file. */
export interface InstructionsRider {
  name: string;
  path: string;
  exists: boolean;
}

/** The instruction file a user set for a target skillshare does not know. */
export interface TargetInstructionsSetup {
  path: string;
  import?: boolean;
}

export type ConvertMethod = 'import' | 'rename' | 'copy';

export interface InstructionsChange {
  path: string;
  status: 'new' | 'modified' | 'removed';
  before: string;
  after: string;
}

export interface SharedInstructionsFile {
  name: string;
  file: string;
  path: string;
  exists: boolean;
  size: number;
  chars: number;
  targets: number;
  locations?: InstructionLocation[]; // other folders the file is put in; an older server leaves it out
}

/** A folder, not a tool's instruction file, that a shared file is put in. */
export interface InstructionLocation {
  path: string; // the folder as stored in config; names the location in the calls below
  file: string; // the file written
  as?: string; // custom file name
  mode: 'import' | 'prepend' | 'append' | 'symlink' | 'copy';
  status: string; // same values as InstructionsAssignment.status
  reason?: 'folder_link' | 'directory';
}

export interface SharedInstructionsTarget {
  name: string;
  path: string;
  import: boolean;
  exists: boolean;
  same_as?: string;
  rider_of?: string; // not a target: reads this target's skills
  linked_shared?: string; // the shared file whose source the target's path is (a link or a tracked copy)
  max_chars?: number;
  assigned: InstructionsAssignment[];
}

/** A copy target rewritten when its shared file was saved. */
export interface SharedCopyResult {
  target: string;
  warnings?: string[];
  error?: string;
}

/** What restoring one target puts back, from the record made when it was attached. */
export interface SharedRestorePreview {
  kind: 'content' | 'delete' | 'link';
  path: string;
  content: string; // the file after restore (kind content)
  current: string; // the file now
  link_to?: string; // kind link
  recorded_at?: string;
  drift: boolean; // edits made after attaching are backed up, not restored
}

export interface ProjectInstructionsReach {
  target: string;
  file: string;
  how: 'direct' | 'fallback' | 'import' | 'link' | 'shadowed' | 'missing';
  reads: boolean;
  shim?: 'import' | 'link';
}

export interface ProjectInstructions {
  path: string;
  exists: boolean;
  content: string;
  size: number;
  targets: ProjectInstructionsReach[];
}
