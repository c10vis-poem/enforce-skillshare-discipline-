import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { api, type Skill, type Target } from '../api/client';
import { I18nProvider } from '../i18n';
import { ToastProvider } from '../components/Toast';
import ResourceDetailPage, { ProjectsList } from './ResourceDetailPage';

vi.mock('../api/client', async (load) => ({ ...await load<typeof import('../api/client')>(), api: {
  getSyncMatrix: vi.fn(), listTargets: vi.fn(), getResource: vi.fn(), listSkills: vi.fn(),
  auditSkill: vi.fn(), diff: vi.fn(), batchUninstall: vi.fn(),
} }));

describe('Resource detail projects', () => {
  // The Skills list counts project targets, so the detail page lists them too, one row per project.
  it('lists each project once and links to its page', async () => {
    vi.mocked(api.listTargets).mockResolvedValue({ targets: [
      { name: 'docs@claude', project: '/work/docs', path: '/work/docs/.claude/skills' },
      { name: 'docs@opencode', project: '/work/docs', path: '/work/docs/.opencode/skills' },
    ] as Target[], sourceSkillCount: 1 });
    vi.mocked(api.getSyncMatrix).mockResolvedValue({ entries: [
      { skill: 'pdf', target: 'docs@claude', status: 'synced', reason: '' },
      { skill: 'pdf', target: 'docs@opencode', status: 'synced', reason: '' },
    ] } as never);
    render(<MemoryRouter><QueryClientProvider client={new QueryClient()}><I18nProvider>
      <ProjectsList resource={{ flatName: 'pdf', kind: 'skill' } as Skill} diffs={[]} statusText={() => 'Linked'} />
    </I18nProvider></QueryClientProvider></MemoryRouter>);
    expect(await screen.findByRole('link', { name: /docs/ })).toHaveAttribute('href', '/projects/%2Fwork%2Fdocs');
  });
});

it('uninstalls only the linked skill from its detail page', async () => {
  const user = userEvent.setup();
  const linked: Skill = {
    name: 'foo', kind: 'skill', flatName: '_dev__foo', relPath: '_dev/foo',
    sourcePath: '/skills/_dev/foo', isInRepo: true, linkName: '_dev', linkTarget: '/checkout',
  };
  vi.mocked(api.getResource).mockResolvedValue({ resource: linked, skillMdContent: '', files: [] });
  vi.mocked(api.listSkills).mockResolvedValue({ resources: [linked, { ...linked, name: 'bar', flatName: '_dev__bar', relPath: '_dev/bar' }] });
  vi.mocked(api.listTargets).mockResolvedValue({ targets: [], sourceSkillCount: 2 });
  vi.mocked(api.getSyncMatrix).mockResolvedValue({ entries: [] } as never);
  vi.mocked(api.diff).mockResolvedValue({ diffs: [] } as never);
  vi.mocked(api.auditSkill).mockResolvedValue({ result: { findings: [] } } as never);
  vi.mocked(api.batchUninstall).mockResolvedValue({ results: [], summary: { succeeded: 1, failed: 0 } });
  render(<MemoryRouter initialEntries={['/skills/_dev__foo']}>
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <I18nProvider><ToastProvider><Routes>
        <Route path="/skills/:name" element={<ResourceDetailPage />} />
        <Route path="/skills" element={<div>Skills</div>} />
      </Routes></ToastProvider></I18nProvider>
    </QueryClientProvider>
  </MemoryRouter>);
  await user.click(await screen.findByRole('button', { name: 'More actions' }));
  expect(screen.queryByRole('menuitem', { name: 'Uninstall Repo' })).toBeNull();
  await user.click(screen.getByRole('menuitem', { name: 'Uninstall' }));
  const dialog = screen.getByRole('dialog');
  expect(within(dialog).getByText('Selected skills are moved out of the linked folder to the trash.')).toBeInTheDocument();
  expect(within(dialog).getByText('foo')).toBeInTheDocument();
  expect(within(dialog).queryByText('bar')).toBeNull();
  await user.click(within(dialog).getByRole('button', { name: 'Uninstall' }));
  await waitFor(() => expect(api.batchUninstall).toHaveBeenCalledWith({ names: ['_dev__foo'], kind: 'skill', force: false }));
});
