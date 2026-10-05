import { Fragment, useRef, useState } from 'react';
import type { CSSProperties, KeyboardEvent, MouseEvent, ReactNode } from 'react';
import { Bot, ChevronDown, ChevronRight, Folder, FolderOpen, GitBranch, Link2, Power, PowerOff, Puzzle } from 'lucide-react';
import type { Skill, SourceLink } from '../../api/client';
import { countLabel } from '../../lib/resourceGrouping';
import { useT } from '../../i18n';
import { formatTrackedRepoName } from '../../lib/resourceNames';
import { insideSelection, skillsUnder } from './tree';
import type { TreeRow } from './tree';

export type SelectMode = 'only' | 'toggle' | 'range';

interface Props {
  rows: TreeRow[];
  selected: ReadonlySet<string>;
  kind: Skill['kind'];
  label: string;
  onSelect: (id: string, mode: SelectMode) => void;
  onToggleFolder: (path: string) => void;
  onOpen: (skill: Skill) => void;
  renderUnlink?: (link: SourceLink) => ReactNode;
  onContextMenu?: (e: MouseEvent, row: TreeRow) => void;
}

/**
 * Explorer-style tree: click selects, Cmd/Ctrl-click adds, Shift-click takes a range,
 * double-click (or Enter) opens a skill. Keyboard follows the WAI-ARIA tree pattern.
 */
export default function SkillTree({ rows, selected, kind, label, onSelect, onToggleFolder, onOpen, renderUnlink, onContextMenu }: Props) {
  const t = useT();
  const refs = useRef(new Map<string, HTMLDivElement>());
  const [focusId, setFocusId] = useState<string | null>(null);
  const ItemIcon = kind === 'agent' ? Bot : Puzzle;

  // Roving tabindex: the focused row, else the first selected one, else the first row.
  const tabId = rows.some((r) => r.id === focusId) ? focusId : (rows.find((r) => selected.has(r.id)) ?? rows[0])?.id;

  const focus = (id: string | undefined) => {
    if (!id) return;
    setFocusId(id);
    refs.current.get(id)?.focus();
  };

  const onKeyDown = (e: KeyboardEvent, i: number) => {
    const row = rows[i];
    const folder = row.type === 'folder' ? row : null;
    switch (e.key) {
      case 'ArrowDown': focus(rows[i + 1]?.id); break;
      case 'ArrowUp': focus(rows[i - 1]?.id); break;
      case 'Home': focus(rows[0]?.id); break;
      case 'End': focus(rows[rows.length - 1]?.id); break;
      case 'ArrowRight':
        if (folder?.collapsed) onToggleFolder(folder.node.path);
        else if (folder && rows[i + 1]?.depth === row.depth + 1) focus(rows[i + 1].id);
        break;
      case 'ArrowLeft':
        if (folder && !folder.collapsed) onToggleFolder(folder.node.path);
        else for (let j = i - 1; j >= 0; j--) if (rows[j].depth === row.depth - 1) { focus(rows[j].id); break; }
        break;
      case ' ': onSelect(row.id, 'toggle'); break;
      case 'Enter':
        if (folder) onToggleFolder(folder.node.path);
        else if (row.type === 'item') onOpen(row.skill);
        break;
      default: return;
    }
    e.preventDefault();
  };

  const onClick = (e: MouseEvent, id: string) => {
    focus(id);
    onSelect(id, e.shiftKey ? 'range' : e.metaKey || e.ctrlKey ? 'toggle' : 'only');
  };

  return (
    <div role="tree" aria-label={label} aria-multiselectable="true" className="ss-tree">
      {rows.map((row, i) => {
        const isSel = selected.has(row.id);
        const inside = !isSel && insideSelection(row, selected);
        const folder = row.type === 'folder' ? row : null;
        const link = kind === 'skill' ? folder?.node.link : undefined;
        const skills = folder ? skillsUnder(folder.node) : [];
        const off = folder ? skills.filter((s) => s.disabled).length : row.type === 'item' && row.skill.disabled ? 1 : 0;
        const dim = folder ? skills.length > 0 && off === skills.length : off === 1;
        return (
          <div
            key={row.id}
            ref={(el) => { if (el) refs.current.set(row.id, el); else refs.current.delete(row.id); }}
            role="treeitem"
            aria-level={row.depth + 1}
            aria-expanded={folder ? !folder.collapsed : undefined}
            aria-selected={isSel}
            tabIndex={row.id === tabId ? 0 : -1}
            className={`ss-tn ${isSel ? 'sel' : inside ? 'in' : ''} ${dim ? 'off' : ''}`}
            style={{ '--d': row.depth } as CSSProperties}
            onMouseDown={(e) => { if (e.shiftKey) e.preventDefault(); }}
            onClick={(e) => onClick(e, row.id)}
            onContextMenu={link ? undefined : (e) => onContextMenu?.(e, row)}
            onDoubleClick={() => { if (row.type === 'item') onOpen(row.skill); }}
            onFocus={() => setFocusId(row.id)}
            onKeyDown={(e) => onKeyDown(e, i)}
          >
            {Array.from({ length: row.depth }, (_, g) => <i key={g} className="gd" style={{ '--g': g } as CSSProperties} />)}
            {folder ? (
              <button
                type="button"
                tabIndex={-1}
                className="cv"
                aria-label={t(folder.collapsed ? 'resources.folder.expand' : 'resources.folder.collapse', { name: folder.names.join('/') })}
                onClick={(e) => { e.stopPropagation(); onToggleFolder(folder.node.path); }}
              >
                {folder.collapsed ? <ChevronRight size={14} /> : <ChevronDown size={14} />}
              </button>
            ) : (
              <span className="cv" />
            )}
            {folder ? (
              link ? <Link2 size={15} className="ic" /> : folder.repo ? <GitBranch size={15} className="ic" /> : folder.collapsed ? <Folder size={15} className="ic" /> : <FolderOpen size={15} className="ic" />
            ) : dim ? (
              // A disabled item says so with its icon; the hover text is for folder counts only.
              <PowerOff size={14} className="ic" role="img" aria-label={t('resources.status.disabled')}>
                <title>{t('resources.status.disabled')}</title>
              </PowerOff>
            ) : (
              <ItemIcon size={14} className="ic" />
            )}
            <span className={`nm ${folder ? 'f' : ''}`}>
              {folder
                ? folder.names.map((n, j) => (
                  <Fragment key={j}>
                    {j > 0 && <span className="sl">/</span>}
                    {folder.repo && !link ? formatTrackedRepoName(n) : n}
                  </Fragment>
                ))
                : row.type === 'item' && row.skill.name}
            </span>
            {folder?.repo && !link && <span className="ss-tag shrink-0">tracked</span>}
            {link && skills.length === 0 && <>
              <span className="min-w-0 truncate font-mono text-xs text-ink-3" title={link.target}>{link.target}</span>
              <span className="text-xs text-ink-3">{link.warning ?? countLabel(t, kind, 0)}</span>
              <span className="ml-auto">{renderUnlink?.(link)}</span>
            </>}
            <span className={`hv ${link ? '!ml-auto' : ''}`}>
              {folder && !(link && skills.length === 0) && <span>{skills.length}</span>}
              {folder && off > 0 && (
                <span className="inline-flex items-center gap-1">
                  <Power size={12} />
                  {off === skills.length ? t('resources.tree.allDisabled') : t('resources.tree.someDisabled', { count: off })}
                </span>
              )}
            </span>
          </div>
        );
      })}
    </div>
  );
}
