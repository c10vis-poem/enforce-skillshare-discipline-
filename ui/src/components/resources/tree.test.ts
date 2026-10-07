import { describe, expect, it } from 'vitest';
import type { Skill } from '../../api/client';
import { buildTree, flattenTree, rangeIds, selectedSkills } from './tree';

const skill = (relPath: string, extra: Partial<Skill> = {}): Skill => {
  const name = relPath.split('/').pop()!;
  return { name, kind: 'skill', flatName: relPath.replace(/\//g, '__'), relPath, sourcePath: '', isInRepo: relPath.startsWith('_'), ...extra };
};

const SKILLS = [
  skill('_repo/plugins/demo/skills/alpha'),
  skill('_repo/plugins/demo/skills/beta'),
  skill('_repo/skills/gamma'),
  skill('local/one'),
  skill('local/two'),
  skill('solo'),
];

const labels = (rows: ReturnType<typeof flattenTree>) =>
  rows.map((r) => `${'  '.repeat(r.depth)}${r.type === 'folder' ? r.names.join(' / ') : r.skill.name}`);

describe('flattenTree', () => {
  it('marks a repo installed with --into as a repo root, not its parent folder', () => {
    const repo = { isInRepo: true, repoPath: 'org/_repo' };
    const rows = flattenTree(buildTree([skill('org/_repo/skills/x', repo)]), new Set(), false);

    expect(rows.filter((r) => r.type === 'folder').map((r) => [r.names.join(' / '), r.type === 'folder' && r.repo]))
      .toEqual([['org', false], ['_repo', true], ['skills', false]]);
  });

  it('merges chains of single-folder folders into one row, but never a tracked repo root', () => {
    expect(labels(flattenTree(buildTree(SKILLS), new Set(), false))).toEqual([
      '_repo',
      '  plugins / demo / skills',
      '    alpha',
      '    beta',
      '  skills',
      '    gamma',
      'local',
      '  one',
      '  two',
      'solo',
    ]);
  });

  it('keys a merged row by its deepest folder, so collapsing it hides that folder', () => {
    const rows = flattenTree(buildTree(SKILLS), new Set(['_repo/plugins/demo/skills']), false);
    expect(labels(rows).slice(0, 3)).toEqual(['_repo', '  plugins / demo / skills', '  skills']);
  });

  it('ignores collapsed folders while filtering', () => {
    const rows = flattenTree(buildTree(SKILLS), new Set(['local']), true);
    expect(labels(rows)).toContain('  one');
  });
});

describe('rangeIds', () => {
  const rows = flattenTree(buildTree(SKILLS), new Set(), false);

  it('takes every visible row between the anchor and the target, in either direction', () => {
    expect(rangeIds(rows, 's:local__two', 'f:_repo/skills')).toEqual(['f:_repo/skills', 's:_repo__skills__gamma', 'f:local', 's:local__one', 's:local__two']);
  });

  it('falls back to the clicked row without a visible anchor', () => {
    expect(rangeIds(rows, 'f:gone', 's:solo')).toEqual(['s:solo']);
  });
});

describe('selectedSkills', () => {
  it('expands a folder to every skill under it, without duplicates', () => {
    const tree = buildTree(SKILLS);
    const byName = new Map(SKILLS.map((s) => [s.flatName, s]));
    const picked = selectedSkills(tree, new Set(['f:_repo', 's:_repo__skills__gamma']), byName);
    expect(picked.map((s) => s.name)).toEqual(['alpha', 'beta', 'gamma']);
  });
});
