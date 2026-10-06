import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import type { Target } from '../api/client';
import { hooksApi } from '../api/hooks';
import { mcpApi } from '../api/mcp';
import { ToastProvider } from '../components/Toast';
import { I18nProvider } from '../i18n';
import SyncPage from './SyncPage';

vi.mock('../api/client', async (load) => ({
  ...await load<typeof import('../api/client')>(),
  api: { listTargets: vi.fn(), diff: vi.fn(), diffExtras: vi.fn(), listLog: vi.fn(), skillsOffPreview: vi.fn(), updateTarget: vi.fn(), sync: vi.fn(), syncExtras: vi.fn() },
}));
vi.mock('../api/mcp', async (load) => ({ ...await load<typeof import('../api/mcp')>(), mcpApi: { list: vi.fn() } }));
vi.mock('../api/hooks', async (load) => ({ ...await load<typeof import('../api/hooks')>(), hooksApi: { list: vi.fn(() => Promise.reject(new Error('offline'))) } }));

const target = (name: string) => ({
  name, path: '/home/me/.agents/skills', mode: 'merge', targetNaming: 'flat', status: 'merged', linkedCount: 3, localCount: 0,
  include: [], exclude: [], expectedSkillCount: 3, skillsEnabled: true,
}) as Target;

describe('Sync page folder conflicts', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listTargets).mockResolvedValue({ targets: [target('codex'), target('universal')], sourceSkillCount: 3 });
    vi.mocked(api.diff).mockResolvedValue({
      diffs: [], ignored_count: 0, ignored_skills: [], ignore_root: '', ignore_repos: [],
      folder_conflicts: [{ path: '/home/me/.agents/skills', targets: ['codex', 'universal'], keep: 'universal', stop: ['codex'] }],
    });
    vi.mocked(api.diffExtras).mockResolvedValue({ extras: [] });
    vi.mocked(api.listLog).mockResolvedValue({ entries: [] } as never);
    vi.mocked(mcpApi.list).mockResolvedValue({ paths: {}, source: { targets: [], servers: {} } } as never);
    vi.mocked(api.skillsOffPreview).mockResolvedValue({ remove: [], keep: [], sharedWith: 'universal' });
    vi.mocked(api.updateTarget).mockResolvedValue({ success: true });
  });

  it('names the targets that undo each other and stops skills for the one to drop', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <QueryClientProvider client={new QueryClient()}><I18nProvider><ToastProvider><SyncPage /></ToastProvider></I18nProvider></QueryClientProvider>
      </MemoryRouter>,
    );
    expect(await screen.findByText(/codex and universal sync skills to the same folder .*\.agents\/skills with different filters, so each sync undoes the other\./)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Stop syncing skills for codex' }));
    await user.click(await screen.findByRole('button', { name: 'Stop syncing' }));
    await waitFor(() => expect(api.updateTarget).toHaveBeenCalledWith('codex', { skills_enabled: false }));
  });
});

describe('Sync page last sync', () => {
  const renderPage = () => render(
    <MemoryRouter>
      <QueryClientProvider client={new QueryClient()}><I18nProvider><ToastProvider><SyncPage /></ToastProvider></I18nProvider></QueryClientProvider>
    </MemoryRouter>,
  );

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listTargets).mockResolvedValue({ targets: [target('codex')], sourceSkillCount: 3 });
    vi.mocked(api.diff).mockResolvedValue({ diffs: [], ignored_count: 0, ignored_skills: [], ignore_root: '', ignore_repos: [] });
    vi.mocked(api.diffExtras).mockResolvedValue({ extras: [] });
    vi.mocked(mcpApi.list).mockResolvedValue({ paths: {}, source: { targets: [], servers: {} } } as never);
  });

  it('names the targets the last sync failed', async () => {
    vi.mocked(api.listLog).mockResolvedValue({ entries: [{ ts: '2026-09-30T00:00:00Z', cmd: 'sync', status: 'partial', args: { targets_total: 3, targets_failed: 2, failed_targets: ['codex', 'cursor'] } }] } as never);
    renderPage();
    expect((await screen.findByText('Failed')).nextElementSibling).toHaveTextContent('codex and cursor');
  });

  it('counts the failed targets when an older entry has no names', async () => {
    vi.mocked(api.listLog).mockResolvedValue({ entries: [{ ts: '2026-09-30T00:00:00Z', cmd: 'sync', status: 'partial', args: { targets_total: 3, targets_failed: 1 } }] } as never);
    renderPage();
    expect((await screen.findByText('Failed')).nextElementSibling).toHaveTextContent('1');
  });
});

describe('Sync page failure diagnostics', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listTargets).mockResolvedValue({ targets: [target('codex')], sourceSkillCount: 3 });
    vi.mocked(api.diff).mockResolvedValue({ diffs: [], ignored_count: 0, ignored_skills: [], ignore_root: '', ignore_repos: [] });
    vi.mocked(api.diffExtras).mockResolvedValue({ extras: [{ name: 'agents', target: '/home/me/.codex/agents', mode: 'copy', synced: false, items: [{ action: 'create', file: 'one.md', reason: 'missing' }] }] });
    vi.mocked(api.listLog).mockResolvedValue({ entries: [{ ts: '2026-09-30T00:00:00Z', cmd: 'sync', status: 'ok', args: { targets_total: 1 } }] } as never);
    vi.mocked(api.sync).mockResolvedValue({ results: [], ignored_count: 0, ignored_skills: [], ignore_root: '', ignore_repos: [] });
    vi.mocked(mcpApi.list).mockResolvedValue({ paths: {}, source: { targets: [], servers: {} } } as never);
    vi.mocked(hooksApi.list).mockRejectedValue(new Error('offline'));
  });

  const renderPage = () => render(
    <MemoryRouter>
      <QueryClientProvider client={new QueryClient()}><I18nProvider><ToastProvider><SyncPage /></ToastProvider></I18nProvider></QueryClientProvider>
    </MemoryRouter>,
  );

  it('summarizes repeated stacks, expands and copies every diagnostic, and scopes the resource OK', async () => {
    const reason = 'ReferenceError: require is not defined in ES module scope';
    const errors = Array.from({ length: 5 }, (_, i) => `${i}.md: extension codex-agents failed: exit status 1\nfile:///extensions/codex-agents:4\nconst fs = require("fs");\n${reason}\n    at file:///extensions/codex-agents:4:12\n    at ModuleJob.run (node:internal/modules/esm/module_job:343:25)`);
    vi.mocked(api.syncExtras).mockResolvedValue({ extras: [{ name: 'agents', targets: [{ target: '/home/me/.codex/agents', mode: 'copy', synced: 0, skipped: 0, pruned: 0, errors }] }] });
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole('button', { name: 'Sync 1 change' }));
    expect(await screen.findByText(reason)).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: '1 target failed' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Last skills and agents sync' }).parentElement).toHaveTextContent('OK');
    expect(screen.getByText(/Skills and agents only/)).toBeInTheDocument();
    const show = screen.getByRole('button', { name: 'Show diagnostics' });
    expect(show).toHaveAttribute('aria-expanded', 'false');
    expect(document.querySelector('pre')).toBeNull();
    await user.click(show);
    const detail = screen.getByLabelText('Show diagnostics');
    expect(detail.textContent).toBe(errors.join('; '));
    expect(detail).toHaveClass('max-h-64', 'overflow-auto');
    await user.click(screen.getByRole('button', { name: 'Copy' }));
    expect(await navigator.clipboard.readText()).toBe(errors.join('; '));
    await user.click(screen.getByRole('button', { name: 'Hide diagnostics' }));
    expect(document.querySelector('pre')).toBeNull();
    expect(screen.getByText(reason)).toBeInTheDocument();
  });

  it('shows a normal short error without an unnecessary disclosure', async () => {
    vi.mocked(api.syncExtras).mockResolvedValue({ extras: [{ name: 'agents', targets: [{ target: '/home/me/.codex/agents', mode: 'copy', synced: 0, skipped: 0, pruned: 0, error: 'permission denied' }] }] });
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole('button', { name: 'Sync 1 change' }));
    expect(await screen.findByText('permission denied')).toBeInTheDocument();
    expect(screen.getByText(/Check the folder's permissions/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Show diagnostics' })).not.toBeInTheDocument();
  });
});

describe('Sync page hooks conflicts', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.listTargets).mockResolvedValue({ targets: [target('codex')], sourceSkillCount: 3 });
    vi.mocked(api.diff).mockResolvedValue({ diffs: [], ignored_count: 0, ignored_skills: [], ignore_root: '', ignore_repos: [] });
    vi.mocked(api.diffExtras).mockResolvedValue({ extras: [] });
    vi.mocked(api.listLog).mockResolvedValue({ entries: [] } as never);
    vi.mocked(mcpApi.list).mockResolvedValue({ paths: {}, source: { targets: [], servers: {} } } as never);
  });

  it('words the unmanaged-hook conflict for this UI and shows all of it on hover', async () => {
    const raw = 'an identical hook exists that Skillshare does not manage; import it or explicitly replace it';
    vi.mocked(hooksApi.list).mockResolvedValue({
      source: { path: '/s.yaml', configPath: '/s.yaml', entries: {} }, targets: [], paths: {}, backups: [], unmanaged: [], previewError: '',
      plan: { revision: 'r', fingerprint: 'f', sourcePath: '/s.yaml', blocked: true, changes: [{ target: 'codex', path: '/home/me/.codex/hooks.json', name: 'codex-stop', action: 'conflict', message: raw }] },
    });
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <QueryClientProvider client={new QueryClient()}><I18nProvider><ToastProvider><SyncPage /></ToastProvider></I18nProvider></QueryClientProvider>
      </MemoryRouter>,
    );
    const reason = await screen.findByText(/The same hook already exists/);
    expect(screen.queryByText(raw)).not.toBeInTheDocument();
    await user.hover(reason);
    expect(await screen.findByRole('tooltip')).toHaveTextContent(/Take over native hooks/);
  });
});
