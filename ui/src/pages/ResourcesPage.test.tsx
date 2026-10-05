import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import type { Skill, SyncMatrixEntry } from '../api/client';
import { I18nProvider } from '../i18n';
import { ToastProvider } from '../components/Toast';
import { byTargetOrProject, splitTargets, syncedByTarget } from '../lib/resourceGrouping';
import ResourcesPage from './ResourcesPage';

vi.mock('../context/AppContext', () => ({ useAppContext: () => ({ isProjectMode: false }) }));
vi.mock('../api/client', async (load) => {
  const actual = await load<typeof import('../api/client')>();
  // Anything the page asks for that a test does not set up answers with an empty object.
  const stubs: Record<string | symbol, unknown> = {};
  return { ...actual, api: new Proxy(stubs, { get: (t, k) => (t[k] ??= vi.fn().mockResolvedValue({})) }) };
});

const skill = (flatName: string): Skill => ({
  name: flatName, kind: 'skill', flatName, relPath: flatName, sourcePath: '', isInRepo: false,
});

const entry = (s: string, target: string, status: SyncMatrixEntry['status']): SyncMatrixEntry =>
  ({ skill: s, target, status, reason: '' });

describe('splitTargets', () => {
  it('groups project targets by project and keeps global tools apart', () => {
    expect(splitTargets(['blog@claude', 'claude', 'codex', 'myapp@claude', 'myapp@codex'])).toEqual({
      global: ['claude', 'codex'],
      projects: [['blog', ['claude']], ['myapp', ['claude', 'codex']]],
    });
  });
});

describe('byTargetOrProject', () => {
  it('merges a project\'s tools into one entry and keeps global targets apart', () => {
    const index = byTargetOrProject(new Map([
      ['claude', new Set(['a'])],
      ['blog@claude', new Set(['a'])],
      ['blog@codex', new Set(['b'])],
    ]));

    expect([...index].map(([k, v]) => [k, [...v]])).toEqual([['claude', ['a']], ['blog@', ['a', 'b']]]);
  });
});

describe('syncedByTarget', () => {
  it('indexes only entries that are synced and belong to the given items', () => {
    const index = syncedByTarget([skill('a'), skill('b')], [
      entry('a', 'claude', 'synced'),
      entry('b', 'claude', 'synced'),
      entry('a', 'cursor', 'not_included'),
      entry('gone', 'codex', 'synced'),
    ]);

    expect([...index.keys()]).toEqual(['claude']);
    expect([...index.get('claude')!]).toEqual(['a', 'b']);
  });
});

/* -- Tree view ----------------------------------- */

const at = (relPath: string, extra: Partial<Skill> = {}): Skill => ({
  name: relPath.split('/').pop()!, kind: 'skill', flatName: relPath.replace(/\//g, '__'), relPath, sourcePath: '',
  isInRepo: relPath.startsWith('_'), ...extra,
});

const SKILLS = [
  at('_repo/plugins/demo/skills/alpha'),
  at('_repo/plugins/demo/skills/beta'),
  at('_repo/skills/gamma'),
  at('local/one'),
  at('local/two', { disabled: true }),
];

function mount(kind: 'skill' | 'agent' = 'skill') {
  render(
    <MemoryRouter initialEntries={['/skills']}>
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <I18nProvider><ToastProvider><ResourcesPage kind={kind} /></ToastProvider></I18nProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

/** The tree row whose name reads `name` ("plugins/demo/skills" for a merged row). */
async function row(name: string) {
  await screen.findAllByRole('treeitem');
  const found = screen.getAllByRole('treeitem').find((el) => el.querySelector('.nm')?.textContent === name);
  if (!found) throw new Error(`no tree row ${name}`);
  return found;
}

describe('Skills tree view', () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('skillshare:skills-view', 'tree');
    vi.clearAllMocks();
    vi.mocked(api.listSkills).mockResolvedValue({ resources: SKILLS } as Awaited<ReturnType<typeof api.listSkills>>);
    vi.mocked(api.diff).mockResolvedValue({ diffs: [] } as unknown as Awaited<ReturnType<typeof api.diff>>);
    vi.mocked(api.listTargets).mockResolvedValue({ targets: [], sourceSkillCount: 0 });
    vi.mocked(api.listTrash).mockResolvedValue({ items: [] } as unknown as Awaited<ReturnType<typeof api.listTrash>>);
    vi.mocked(api.getSyncMatrix).mockResolvedValue({ entries: [] } as unknown as Awaited<ReturnType<typeof api.getSyncMatrix>>);
    vi.mocked(api.batchToggleResources).mockResolvedValue({ results: [], summary: { updated: 1, unchanged: 0, failed: 0 } } as Awaited<ReturnType<typeof api.batchToggleResources>>);
    vi.mocked(api.batchSetTargets).mockResolvedValue({ updated: 2, skipped: 0, errors: [] });
    vi.mocked(api.availableTargets).mockResolvedValue({ targets: [{ name: 'claude', installed: true }] } as Awaited<ReturnType<typeof api.availableTargets>>);
  });

  it('turns every skill of a partly enabled folder on from its switch', async () => {
    mount();
    fireEvent.click(await row('local'));
    fireEvent.click(screen.getByRole('switch', { name: 'local' }));
    await waitFor(() => expect(api.batchToggleResources).toHaveBeenCalledWith(['local__one', 'local__two'], true, 'skill'));
  });

  it('turns every skill of a fully enabled folder off, without asking first', async () => {
    mount();
    fireEvent.click(await row('repo'));
    const sw = screen.getByRole('switch', { name: 'repo' });
    expect(sw).toHaveAttribute('aria-checked', 'true');
    fireEvent.click(sw);
    await waitFor(() => expect(api.batchToggleResources).toHaveBeenCalledWith(
      ['_repo__plugins__demo__skills__alpha', '_repo__plugins__demo__skills__beta', '_repo__skills__gamma'], false, 'skill',
    ));
  });

  it('shows only the latest toast after quick off/on clicks', async () => {
    mount();
    fireEvent.click(await row('repo'));
    const sw = screen.getByRole('switch', { name: 'repo' });
    fireEvent.click(sw);
    await waitFor(() => expect(sw).not.toBeDisabled());
    fireEvent.click(sw);
    await waitFor(() => expect(api.batchToggleResources).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(sw).not.toBeDisabled());
    const toasts = [...document.querySelectorAll('[data-toast-container] .ss-toast')].map((el) => el.textContent);
    const lastEnable = vi.mocked(api.batchToggleResources).mock.calls[1][1];
    expect(toasts).toEqual([expect.stringMatching(lastEnable ? /^Enabled/ : /^Disabled/)]);
  });

  it('marks a disabled skill with its icon instead of hover text', async () => {
    mount();
    expect(within(await row('two')).getByRole('img', { name: 'Disabled' })).toBeInTheDocument();
    expect(within(await row('one')).queryByRole('img', { name: 'Disabled' })).toBeNull();
    expect(within(await row('two')).queryByText('Disabled', { ignore: 'title' })).toBeNull();
  });

  it('collapses every folder from one button, which then expands them again', async () => {
    mount();
    await row('one');
    fireEvent.click(screen.getByRole('button', { name: 'Collapse all' }));
    expect(screen.getAllByRole('treeitem').map((el) => el.querySelector('.nm')?.textContent)).toEqual(['repo', 'local']);
    fireEvent.click(screen.getByRole('button', { name: 'Expand all' }));
    expect(await row('one')).toBeInTheDocument();
  });

  it('keeps a plain folder name as is in the detail pane path', async () => {
    vi.mocked(api.listSkills).mockResolvedValue({ resources: [at('notes__2024/deep/x'), at('notes__2024/deep/y')] } as Awaited<ReturnType<typeof api.listSkills>>);
    mount();
    fireEvent.click(await row('x'));
    expect(await screen.findByText('notes__2024 / deep /')).toBeInTheDocument();
  });

  it('selects the visible range on Shift-click', async () => {
    mount();
    fireEvent.click(await row('gamma'));
    fireEvent.click(await row('one'), { shiftKey: true });
    expect(screen.getAllByRole('treeitem', { selected: true }).map((el) => el.querySelector('.nm')?.textContent)).toEqual(['gamma', 'local', 'one']);
    expect(screen.getByRole('heading', { name: '3 skills selected' })).toBeInTheDocument();
  });

  it('sets targets for a folder inside a tracked repo', async () => {
    const user = userEvent.setup();
    mount();
    await user.click(await row('plugins/demo/skills'));
    await user.click(screen.getByRole('button', { name: 'Set targets' }));
    await user.click(within(await screen.findByRole('menu')).getByRole('menuitem', { name: 'claude' }));
    await waitFor(() => expect(api.batchSetTargets).toHaveBeenCalledWith('_repo/plugins/demo/skills', 'claude'));
  });

  it('remembers the tree width set with the divider', async () => {
    mount();
    const divider = await screen.findByRole('separator');
    fireEvent.keyDown(divider, { key: 'ArrowRight' });
    expect(divider).toHaveAttribute('aria-valuenow', '396');
    expect(localStorage.getItem('skillshare:tree-width')).toBe('396');
  });
});

describe('Link folder', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
    vi.mocked(api.listSkills).mockResolvedValue({ resources: [] });
    vi.mocked(api.getConfig).mockResolvedValue({ config: { FollowSourceLinks: false }, raw: '' });
    vi.mocked(api.createSourceLink).mockResolvedValue({ path: '/source/_team', target: '/work/team', kind: 'symlink', warning: '' });
  });

  it('opens the dialog on the skills page', async () => {
    mount();
    fireEvent.click(await screen.findByRole('button', { name: 'Link folder' }));
    expect(screen.getByRole('dialog', { name: 'Link folder' })).toBeInTheDocument();
    expect(screen.getByLabelText('Folder path')).toBeInTheDocument();
    expect(await screen.findByRole('checkbox', { name: /Enable follow_source_links/ })).not.toBeChecked();
  });

  it('posts the path, optional name and explicit enable flag and refreshes the list', async () => {
    mount();
    fireEvent.click(await screen.findByRole('button', { name: 'Link folder' }));
    fireEvent.change(screen.getByLabelText('Folder path'), { target: { value: '/work/team' } });
    fireEvent.change(screen.getByLabelText('Link name (optional)'), { target: { value: '_team' } });
    fireEvent.click(await screen.findByRole('checkbox', { name: /Enable follow_source_links/ }));
    const calls = vi.mocked(api.listSkills).mock.calls.length;
    fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Link folder' }));
    await waitFor(() => expect(api.createSourceLink).toHaveBeenCalledWith({ path: '/work/team', name: '_team', enable: true }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(api.listSkills).mock.calls.length).toBeGreaterThan(calls);
  });

  it('renders the HTTP 400 guard message verbatim', async () => {
    const { ApiError } = await import('../api/client');
    vi.mocked(api.createSourceLink).mockRejectedValue(new ApiError(400, 'target overlaps sync target /tools/skills'));
    mount();
    fireEvent.click(await screen.findByRole('button', { name: 'Link folder' }));
    fireEvent.change(screen.getByLabelText('Folder path'), { target: { value: '/tools/skills' } });
    await screen.findByRole('checkbox', { name: /Enable follow_source_links/ });
    fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Link folder' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('target overlaps sync target /tools/skills');
    expect(screen.getByLabelText('Folder path')).toHaveValue('/tools/skills');
  });

  it('hides the enable checkbox when following is already on and shows the warning', async () => {
    vi.mocked(api.getConfig).mockResolvedValue({ config: { FollowSourceLinks: true }, raw: '' });
    vi.mocked(api.createSourceLink).mockResolvedValue({ path: '/source/_team', target: '/work/team', kind: 'symlink', warning: 'target is not a git checkout' });
    mount();
    fireEvent.click(await screen.findByRole('button', { name: 'Link folder' }));
    fireEvent.change(screen.getByLabelText('Folder path'), { target: { value: '/work/team' } });
    const submit = within(screen.getByRole('dialog')).getByRole('button', { name: 'Link folder' });
    await waitFor(() => expect(submit).not.toBeDisabled());
    expect(screen.queryByRole('checkbox', { name: /Enable follow_source_links/ })).toBeNull();
    fireEvent.click(submit);
    await waitFor(() => expect(api.createSourceLink).toHaveBeenCalledWith({ path: '/work/team', enable: false }));
    expect(await screen.findByText('target is not a git checkout')).toBeInTheDocument();
  });

  it('does not offer Link folder on the agents page', async () => {
    mount('agent');
    await screen.findByRole('heading', { name: 'Agents' });
    expect(screen.queryByRole('button', { name: 'Link folder' })).toBeNull();
  });
});

/* -- Folders ------------------------------------- */

describe('Unlink folder', () => {
  const LINKED = [
    at('team/plugins/skills/alpha', { isInRepo: false, linkName: 'team', linkTarget: '/work/team' }),
    at('plain/plugins/skills/beta', { isInRepo: false }),
  ];
  const linkedHeader = () => [...document.querySelectorAll<HTMLElement>('.ss-gh, .ss-gl')].find((g) => g.querySelector('b')?.textContent === 'team')!;
  const openUnlink = async () => {
    await screen.findByText('linked');
    const user = userEvent.setup();
    await user.click(within(linkedHeader()).getByRole('button', { name: 'Repo actions' }));
    await user.click(screen.getByRole('menuitem', { name: 'Unlink…' }));
    return screen.getByRole('dialog', { name: 'Unlink team?' });
  };

  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
    vi.mocked(api.listSkills).mockResolvedValue({ resources: LINKED });
    vi.mocked(api.removeSourceLink).mockResolvedValue({ success: true, name: 'team' });
    vi.mocked(api.batchUninstall).mockResolvedValue({ results: [], summary: { succeeded: 1, failed: 0 } });
    vi.mocked(api.diff).mockResolvedValue({ diffs: [] } as unknown as Awaited<ReturnType<typeof api.diff>>);
    vi.mocked(api.listTargets).mockResolvedValue({ targets: [], sourceSkillCount: 0 });
    vi.mocked(api.listTrash).mockResolvedValue({ items: [] } as unknown as Awaited<ReturnType<typeof api.listTrash>>);
    vi.mocked(api.getSyncMatrix).mockResolvedValue({ entries: [] } as unknown as Awaited<ReturnType<typeof api.getSyncMatrix>>);
  });

  it('shows one linked badge, the target and a menu instead of an inline Unlink button', async () => {
    vi.mocked(api.listSkills).mockResolvedValue({ resources: [{ ...LINKED[0], isInRepo: false, linkIsRepo: true }, LINKED[1]] });
    mount();
    await screen.findByText('linked');
    const linked = linkedHeader();
    expect(within(linked).getAllByText('linked')).toHaveLength(1);
    expect(within(linked).queryByText('tracked')).toBeNull();
    expect(linked.querySelector('.lucide-link-2')).toBeInTheDocument();
    expect(within(linked).getByText('/work/team')).toHaveAttribute('title', '/work/team');
    expect(within(linked).getByRole('button', { name: 'Update repo' })).toBeInTheDocument();
    expect(within(linked).getByRole('button', { name: 'Repo actions' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Unlink' })).toBeNull();
    const plain = [...document.querySelectorAll<HTMLElement>('.ss-gh')].find((g) => g.querySelector('b')?.textContent === 'Local')!;
    expect(within(plain).queryByText('linked')).toBeNull();
    expect(within(plain).queryByRole('button', { name: 'Repo actions' })).toBeNull();
  });

  it('replaces Uninstall repo with Unlink in the linked group menu and preserves plain repo actions', async () => {
    const user = userEvent.setup();
    vi.mocked(api.listSkills).mockResolvedValue({ resources: [{ ...LINKED[0], isInRepo: true, linkIsRepo: true }, at('_repo/skills/gamma')] });
    mount();
    await screen.findByText('linked');
    await user.click(within(linkedHeader()).getByRole('button', { name: 'Repo actions' }));
    expect(screen.getByRole('menuitem', { name: 'Unlink…' })).toHaveClass('dng');
    expect(screen.queryByRole('menuitem', { name: 'Uninstall repo' })).toBeNull();
    expect(api.removeSourceLink).not.toHaveBeenCalled();
    await user.keyboard('{Escape}');
    const plain = [...document.querySelectorAll<HTMLElement>('.ss-gh')].find((g) => g.querySelector('b')?.textContent === 'repo')!;
    await user.click(within(plain).getByRole('button', { name: 'Repo actions' }));
    expect(screen.getByRole('menuitem', { name: 'Uninstall repo' })).toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: 'Unlink…' })).toBeNull();
  });

  it.each(['list', 'cards', 'tree'])('hides Update repo for a non-checkout underscore link in %s view', async (view) => {
    localStorage.setItem('skillshare:skills-view', view);
    vi.mocked(api.listSkills).mockResolvedValue({ resources: [at('_team/skills/alpha', {
      isInRepo: true, linkName: '_team', linkTarget: '/work/team', linkIsRepo: false,
    })] });
    mount();
    if (view === 'tree') fireEvent.click(await row('_team'));
    else await screen.findByText('linked');
    expect(screen.queryByRole('button', { name: 'Update repo' })).toBeNull();
    expect(screen.queryByText('tracked')).toBeNull();
  });

  it('opens the linked group menu on list right-click before showing confirmation', async () => {
    mount();
    await screen.findByText('linked');
    fireEvent.contextMenu(linkedHeader());
    await userEvent.setup().click(screen.getByRole('menuitem', { name: 'Unlink…' }));
    expect(screen.getByRole('dialog', { name: 'Unlink team?' })).toBeInTheDocument();
    expect(api.removeSourceLink).not.toHaveBeenCalled();
  });

  it('keeps the linked tree row compact and puts secondary Unlink in its detail pane', async () => {
    localStorage.setItem('skillshare:skills-view', 'tree');
    vi.mocked(api.listSkills).mockResolvedValue({ resources: [{ ...LINKED[0], isInRepo: false, linkIsRepo: true }, LINKED[1]] });
    mount();
    const linked = await row('team');
    expect(within(linked).getByText('linked')).toBeInTheDocument();
    expect(within(linked).queryByText('tracked')).toBeNull();
    expect(linked.querySelector('.lucide-link-2')).toBeInTheDocument();
    expect(within(linked).queryByText('/work/team')).toBeNull();
    expect(within(linked).queryByRole('button', { name: 'Unlink' })).toBeNull();
    expect(within(await row('plain/plugins/skills')).queryByText('linked')).toBeNull();
    fireEvent.click(linked);
    expect(screen.getByText('/work/team')).toBeInTheDocument();
    expect(screen.queryByText('tracked')).toBeNull();
    expect(screen.getByRole('button', { name: 'Update repo' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Uninstall repo' })).toBeNull();
    const unlink = screen.getByRole('button', { name: 'Unlink' });
    expect(unlink).not.toHaveClass('dng');
    expect(unlink.querySelector('.lucide-unlink-2')).toBeInTheDocument();
    fireEvent.click(unlink);
    expect(screen.getByRole('dialog', { name: 'Unlink team?' })).toBeInTheDocument();
    expect(api.removeSourceLink).not.toHaveBeenCalled();
  });

  it('opens the same unlink menu on tree right-click without sending a request', async () => {
    localStorage.setItem('skillshare:skills-view', 'tree');
    mount();
    fireEvent.contextMenu(await row('team'));
    expect(screen.queryByRole('menuitem', { name: 'Uninstall repo' })).toBeNull();
    await userEvent.setup().click(screen.getByRole('menuitem', { name: 'Unlink…' }));
    expect(screen.getByRole('dialog', { name: 'Unlink team?' })).toBeInTheDocument();
    expect(api.removeSourceLink).not.toHaveBeenCalled();
  });

  it('groups linked skills by their link root in folder mode with the same menu', async () => {
    const user = userEvent.setup();
    mount();
    await user.click(await screen.findByRole('button', { name: 'Group and sort' }));
    await user.click(screen.getByRole('menuitemradio', { name: 'Folder' }));
    const linked = linkedHeader();
    expect(within(linked).queryByRole('button', { name: 'Unlink' })).toBeNull();
    expect(within(linked).queryByRole('button', { name: 'Update repo' })).toBeNull();
    const plain = [...document.querySelectorAll<HTMLElement>('.ss-gh')].find((g) => g.querySelector('b')?.textContent === 'plain/plugins/skills')!;
    expect(within(plain).queryByText('linked')).toBeNull();
    expect(within(plain).queryByRole('button', { name: 'Repo actions' })).toBeNull();
    await openUnlink();
    expect(api.removeSourceLink).not.toHaveBeenCalled();
  });

  it('offers the same unlink menu and confirmation from the cards group', async () => {
    localStorage.setItem('skillshare:skills-view', 'cards');
    mount();
    expect(await openUnlink()).toBeInTheDocument();
    expect(api.removeSourceLink).not.toHaveBeenCalled();
  });

  it('confirms the target, affected skills and trash recovery before deleting', async () => {
    mount();
    const dialog = await openUnlink();
    expect(within(dialog).getByText('Target')).toBeInTheDocument();
    expect(within(dialog).getByText('/work/team')).toBeInTheDocument();
    expect(within(dialog).getByText('Skills')).toBeInTheDocument();
    expect(within(dialog).getByText('alpha')).toBeInTheDocument();
    expect(within(dialog).getByText('These skills leave every target on the next sync.')).toBeInTheDocument();
    expect(within(dialog).getByText('The link is removed from the skills source, and the folder it points at is not touched.')).toBeInTheDocument();
    expect(within(dialog).getByText('The link goes to trash and can be restored from the Trash page.')).toBeInTheDocument();
    expect(api.removeSourceLink).not.toHaveBeenCalled();
    const calls = vi.mocked(api.listSkills).mock.calls.length;
    fireEvent.click(within(dialog).getByRole('button', { name: 'Unlink' }));
    await waitFor(() => expect(api.removeSourceLink).toHaveBeenCalledWith('team'));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(api.listSkills).mock.calls.length).toBeGreaterThan(calls);
    expect(screen.getByText('Unlinked team')).toBeInTheDocument();
  });

  it('renders the unlink HTTP 400 message verbatim and keeps the dialog open', async () => {
    const { ApiError } = await import('../api/client');
    vi.mocked(api.removeSourceLink).mockRejectedValue(new ApiError(400, 'team is not a link'));
    mount();
    const dialog = await openUnlink();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Unlink' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('team is not a link');
    expect(dialog).toBeInTheDocument();
  });

  it.each(['actions', 'right-click', 'cards actions', 'cards right-click', 'tree right-click'])('confirms single linked skill uninstall from %s and sends only that skill name', async (action) => {
    const user = userEvent.setup();
    vi.mocked(api.listSkills).mockResolvedValue({ resources: [
      { ...LINKED[0], isInRepo: true, linkIsRepo: true },
      at('team/skills/second', { isInRepo: true, linkName: 'team', linkTarget: '/work/team' }),
    ] });
    if (action === 'tree right-click') localStorage.setItem('skillshare:skills-view', 'tree');
    else if (action.startsWith('cards')) localStorage.setItem('skillshare:skills-view', 'cards');
    mount();
    if (action === 'tree right-click') fireEvent.contextMenu(await row('alpha'));
    else {
      const item = (await screen.findByRole('link', { name: 'alpha' })).closest('.ss-r, .ss-tile')!;
      if (action.includes('right-click')) fireEvent.contextMenu(item);
      else await user.click(within(item as HTMLElement).getByRole('button', { name: 'Actions' }));
    }
    expect(screen.queryByRole('menuitem', { name: 'Uninstall repo' })).toBeNull();
    await user.click(screen.getByRole('menuitem', { name: 'Uninstall' }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText('Selected skills are moved out of the linked folder to the trash.')).toBeInTheDocument();
    expect(within(dialog).getByText('alpha')).toBeInTheDocument();
    expect(within(dialog).queryByText('second')).toBeNull();
    expect(api.batchUninstall).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Uninstall' }));
    await waitFor(() => expect(api.batchUninstall).toHaveBeenCalledWith({ names: ['team__plugins__skills__alpha'], kind: 'skill', force: false }));
  });

  it('preserves whole-repo uninstall confirmation for a plain tracked skill', async () => {
    const user = userEvent.setup();
    vi.mocked(api.listSkills).mockResolvedValue({ resources: [at('_repo/skills/gamma'), at('_repo/skills/other')] });
    mount();
    const item = (await screen.findByRole('link', { name: 'gamma' })).closest('.ss-r')!;
    await user.click(within(item as HTMLElement).getByRole('button', { name: 'Actions' }));
    await user.click(screen.getByRole('menuitem', { name: 'Uninstall repo' }));
    expect(within(screen.getByRole('dialog')).getByText(/Whole repo/)).toBeInTheDocument();
    expect(api.batchUninstall).not.toHaveBeenCalled();
  });

  it('does not show link controls for agents', async () => {
    vi.mocked(api.listSkills).mockResolvedValue({ resources: LINKED.map((s) => ({ ...s, kind: 'agent' })) });
    mount('agent');
    await screen.findByRole('heading', { name: 'Agents' });
    expect(screen.queryByText('linked')).toBeNull();
    expect(screen.queryByRole('button', { name: 'Unlink' })).toBeNull();
  });
});

describe('Skills list folders', () => {
  const FOLDERED = [at('frontend/react/hooks'), at('frontend/react/router'), at('frontend/vue'), at('solo'), at('_repo/skills/gamma')];

  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('skillshare:skills-view', 'list');
    vi.clearAllMocks();
    HTMLElement.prototype.scrollIntoView = vi.fn();
    vi.mocked(api.listSkills).mockResolvedValue({ resources: FOLDERED } as Awaited<ReturnType<typeof api.listSkills>>);
    vi.mocked(api.diff).mockResolvedValue({ diffs: [] } as unknown as Awaited<ReturnType<typeof api.diff>>);
    vi.mocked(api.listTargets).mockResolvedValue({ targets: [], sourceSkillCount: 0 });
    vi.mocked(api.listTrash).mockResolvedValue({ items: [] } as unknown as Awaited<ReturnType<typeof api.listTrash>>);
    vi.mocked(api.getSyncMatrix).mockResolvedValue({ entries: [] } as unknown as Awaited<ReturnType<typeof api.getSyncMatrix>>);
  });

  /** Opens the toolbar select whose prefix reads `prefix` and picks `option`. */
  async function choose(prefix: string, option: string) {
    const user = userEvent.setup();
    const box = (await screen.findAllByRole('combobox')).find((el) => el.textContent?.startsWith(prefix));
    if (!box) throw new Error(`no ${prefix} select`);
    await user.click(box);
    await user.click(await screen.findByRole('option', { name: option }));
  }

  const names = () => [...document.querySelectorAll('.ss-r .nm')].map((el) => el.textContent);

  it('shows only the chosen folder\'s items', async () => {
    mount();
    await choose('Folder', 'frontend/react (2)');
    expect(names()).toEqual(['hooks', 'router']);
  });

  it('shows an unset filter by its name alone', async () => {
    mount();
    expect((await screen.findAllByRole('combobox')).map((el) => el.textContent)).toEqual(['Source', 'Status', 'Folder']);
  });

  it('clears a filter from its chip', async () => {
    mount();
    await choose('Folder', 'frontend/react (2)');
    fireEvent.click(screen.getByRole('button', { name: 'Clear Folder' }));
    expect(names()).toHaveLength(FOLDERED.length);
  });

  it('groups by folder with the root first, then folders A to Z', async () => {
    const user = userEvent.setup();
    mount();
    await user.click(await screen.findByRole('button', { name: 'Group and sort' }));
    await user.click(screen.getByRole('menuitemradio', { name: 'Folder' }));
    expect([...document.querySelectorAll('.ss-gh b')].map((el) => el.textContent)).toEqual(['Root', 'frontend', 'frontend/react', 'repo']);
  });
});
