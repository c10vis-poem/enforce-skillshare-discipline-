import type { Skill, SourceLink, SyncMatrixEntry } from '../api/client';
import type { useT } from '../i18n';
import { resolveSource, type SourceType } from '../components/SourceBadge';
import { parseRemoteURL } from './parseRemoteURL';
import { folderOf, formatTrackedRepoName } from './resourceNames';

export type SourceFilter = 'all' | SourceType;
export type SortType = 'name-asc' | 'name-desc' | 'newest' | 'oldest';

export const SOURCE_ORDER: SourceType[] = ['tracked', 'github', 'remote', 'local'];
// Source names stay in English, like the CLI.
export const SOURCE_LABEL: Record<SourceFilter, string> = { all: 'All', tracked: 'Tracked', github: 'GitHub', remote: 'Remote', local: 'Local' };

/** "1 skill" / "3 agents". The i18n layer has no plural rules, so pick the key by count. */
export function countLabel(t: ReturnType<typeof useT>, kind: Skill['kind'], count: number): string {
  return t(`resources.count.${kind}${count === 1 ? '' : 's'}`, { count });
}

/**
 * Which of `items` each target actually receives, keyed by target name.
 * Reads the sync matrix — the same source the Targets column renders — so a
 * filter result always matches the icons the rows show.
 */
export function syncedByTarget(items: Skill[], matrix: SyncMatrixEntry[]): Map<string, Set<string>> {
  const names = new Set(items.map((s) => s.flatName));
  const byTarget = new Map<string, Set<string>>();
  for (const e of matrix) {
    if (e.status !== 'synced' || !names.has(e.skill)) continue;
    let set = byTarget.get(e.target);
    if (!set) byTarget.set(e.target, (set = new Set()));
    set.add(e.skill);
  }
  return byTarget;
}

/** The project a target name belongs to, or '' for a global target. */
export const projectOf = (name: string) => (name.includes('@') ? name.slice(0, name.lastIndexOf('@')) : '');

/**
 * Folds each project's targets into one filter entry keyed `<project>@`, holding what any of its
 * tools receives; global targets keep their own entries.
 */
export function byTargetOrProject(byTarget: Map<string, Set<string>>): Map<string, Set<string>> {
  const out = new Map<string, Set<string>>();
  for (const [name, names] of byTarget) {
    const project = projectOf(name);
    const key = project ? `${project}@` : name;
    out.set(key, new Set([...(out.get(key) ?? []), ...names]));
  }
  return out;
}

// Group key for sorting: tracked repo name or first dir segment.
function sortGroup(s: Skill): string {
  const slash = s.relPath.indexOf('/');
  return slash > 0 ? s.relPath.slice(0, slash) : '';
}

export function sortSkills(skills: Skill[], sortType: SortType): Skill[] {
  const byName = (a: Skill, b: Skill) => sortGroup(a).localeCompare(sortGroup(b)) || a.name.localeCompare(b.name);
  const byDate = (dir: 1 | -1) => (a: Skill, b: Skill) => {
    if (!a.installedAt && !b.installedAt) return byName(a, b);
    if (!a.installedAt) return 1;
    if (!b.installedAt) return -1;
    return dir * (new Date(a.installedAt).getTime() - new Date(b.installedAt).getTime());
  };
  const sorted = [...skills];
  switch (sortType) {
    case 'name-asc': return sorted.sort(byName);
    case 'name-desc': return sorted.sort((a, b) => sortGroup(a).localeCompare(sortGroup(b)) || b.name.localeCompare(a.name));
    case 'newest': return sorted.sort(byDate(-1));
    case 'oldest': return sorted.sort(byDate(1));
  }
}

export function repoOf(s: Skill): string | undefined {
  return s.isInRepo ? s.relPath.split('/')[0] : undefined;
}

export function sourceLinkOf(s: Skill): SourceLink | undefined {
  return s.kind === 'skill' && s.linkName && s.linkTarget ? { name: s.linkName, target: s.linkTarget, isRepo: !!s.linkIsRepo } : undefined;
}

/** Parent folder shown under the name. Inside a repo group the repo prefix is already in the header. */
export function parentPath(s: Skill, inGroup = false): string {
  const i = s.relPath.lastIndexOf('/');
  if (i <= 0) return '';
  const dir = s.relPath.slice(0, i);
  const repo = inGroup ? sourceLinkOf(s)?.name ?? repoOf(s) : undefined;
  if (repo) return dir === repo ? '' : dir.slice(repo.length + 1);
  return formatTrackedRepoName(dir);
}

export function sourceName(s: Skill): string {
  const link = sourceLinkOf(s);
  if (link) return formatTrackedRepoName(link.name);
  const repo = repoOf(s);
  if (repo) return formatTrackedRepoName(repo);
  if (s.source) return parseRemoteURL(s.source)?.ownerRepo ?? s.source;
  return SOURCE_LABEL.local;
}

/* -- Source groups -------------------------------- */

export interface Group { key: string; source: SourceType; repo?: string; link?: SourceLink; items: Skill[] }

export function groupBySource(items: Skill[]): Group[] {
  const groups = new Map<string, Group>();
  for (const s of items) {
    const source = resolveSource(s.type, s.isInRepo);
    const link = sourceLinkOf(s);
    const repo = link ? (link.isRepo ? link.name : undefined) : repoOf(s);
    const key = link ? `link:${link.name}` : repo ?? source;
    if (!groups.has(key)) groups.set(key, { key, source, repo, link, items: [] });
    groups.get(key)!.items.push(s);
  }
  return [...groups.values()].sort((a, b) => SOURCE_ORDER.indexOf(a.source) - SOURCE_ORDER.indexOf(b.source) || a.key.localeCompare(b.key));
}

/* -- Folder groups -------------------------------- */

export interface FolderGroup { key: string; repo: boolean; link?: SourceLink; items: Skill[] }

/** Root first, then folder name A→Z; items keep the order they came in (the current sort). */
export function groupByFolder(items: Skill[]): FolderGroup[] {
  const groups = new Map<string, FolderGroup>();
  for (const s of items) {
    const link = sourceLinkOf(s);
    const key = link?.name ?? folderOf(s);
    if (!groups.has(key)) groups.set(key, { key, repo: link ? link.isRepo : !!repoOf(s), link, items: [] });
    groups.get(key)!.items.push(s);
  }
  return [...groups.values()].sort((a, b) => formatTrackedRepoName(a.key).localeCompare(formatTrackedRepoName(b.key)));
}

/** Cut groups down to the first `limit` items, keeping headers only for groups that still show something. */
export function limitGroups<G extends { items: Skill[] }>(groups: G[], limit: number): G[] {
  const out: G[] = [];
  let left = limit;
  for (const g of groups) {
    if (left <= 0) break;
    out.push({ ...g, items: g.items.slice(0, left) });
    left -= g.items.length;
  }
  return out;
}

/** Splits target names into global tools and projects; `myapp@claude` is claude in the myapp project. */
export function splitTargets(names: string[]): { global: string[]; projects: [string, string[]][] } {
  const global: string[] = [];
  const projects = new Map<string, string[]>();
  for (const n of names) {
    const i = n.lastIndexOf('@');
    if (i < 0) {
      global.push(n);
      continue;
    }
    const project = n.slice(0, i);
    projects.set(project, [...(projects.get(project) ?? []), n.slice(i + 1)]);
  }
  return { global, projects: [...projects] };
}
