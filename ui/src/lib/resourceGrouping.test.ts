import { describe, expect, it } from 'vitest';
import type { Skill } from '../api/client';
import { groupByFolder, groupBySource, limitGroups, sortSkills } from './resourceGrouping';

const skill = (relPath: string, extra: Partial<Skill> = {}): Skill => ({
  name: relPath.split('/').pop()!, kind: 'skill', flatName: relPath.replace(/\//g, '__'), relPath, sourcePath: '',
  isInRepo: false, ...extra,
});

describe('sortSkills', () => {
  it('puts skills without an install date last when sorting by newest', () => {
    const sorted = sortSkills([
      skill('undated'),
      skill('old', { installedAt: '2024-01-01T00:00:00Z' }),
      skill('new', { installedAt: '2025-01-01T00:00:00Z' }),
    ], 'newest');

    expect(sorted.map((s) => s.name)).toEqual(['new', 'old', 'undated']);
  });
});

describe('groupBySource', () => {
  it('orders tracked repos before local skills and keeps one group per repo', () => {
    const groups = groupBySource([
      skill('solo'),
      skill('_repo-b/x', { isInRepo: true, type: 'tracked' }),
      skill('_repo-a/y', { isInRepo: true, type: 'tracked' }),
      skill('_repo-a/z', { isInRepo: true, type: 'tracked' }),
    ]);

    expect(groups.map((g) => [g.key, g.items.length])).toEqual([['_repo-a', 2], ['_repo-b', 1], ['local', 1]]);
  });

  it('groups a repo installed with --into under its full repo path', () => {
    const repo = { isInRepo: true, type: 'tracked', repoPath: 'org/_repo' };
    const groups = groupBySource([skill('org/_repo/skills/x', repo), skill('org/_repo/skills/y', repo)]);

    expect(groups.map((g) => [g.key, g.items.length])).toEqual([['org/_repo', 2]]);
  });
});

describe('groupByFolder', () => {
  it('lists the root group first and keeps item order within a group', () => {
    const groups = groupByFolder([skill('work/b'), skill('root'), skill('work/a')]);

    expect(groups.map((g) => [g.key, g.items.map((s) => s.name)])).toEqual([['', ['root']], ['work', ['b', 'a']]]);
  });
});

describe('limitGroups', () => {
  it('cuts the last visible group and drops groups past the limit', () => {
    const groups = [{ items: [skill('a'), skill('b')] }, { items: [skill('c'), skill('d')] }, { items: [skill('e')] }];

    expect(limitGroups(groups, 3).map((g) => g.items.map((s) => s.name))).toEqual([['a', 'b'], ['c']]);
  });
});
