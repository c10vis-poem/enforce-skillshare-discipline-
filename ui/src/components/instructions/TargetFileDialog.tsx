import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { X } from 'lucide-react';
import { api } from '../../api/client';
import AgentIcon from '../AgentIcon';
import Button from '../Button';
import CodeView from '../CodeView';
import DialogShell from '../DialogShell';
import Spinner from '../Spinner';
import { useT } from '../../i18n';
import { queryKeys } from '../../lib/queryKeys';
import { shortenHome } from '../../lib/paths';
import { instructionsErrorMessage } from './instructionsView';
import { BoxHeader, InstructionsPreview } from './ViewTabs';

/**
 * A read-only look at a target's instruction file, opened from its row on the
 * AGENTS.md tab: what the tool reads right now, with the shared parts in place.
 * Editing stays on the target page, which the footer links to.
 */
export default function TargetFileDialog({ target, path, names, onClose }: {
  target: string;
  path: string;
  /** Shared file names, so the preview can name the imports. */
  names: string[];
  onClose: () => void;
}) {
  const t = useT();
  const [view, setView] = useState<'preview' | 'source'>('preview');
  const { data, error, isPending } = useQuery({ queryKey: queryKeys.instructions.target(target), queryFn: () => api.getTargetInstructions(target) });
  const content = data?.content ?? '';
  const label = t('instructions.peek.label', { target });

  return (
    <DialogShell open onClose={onClose} padding="none" ariaLabel={label} className="!max-w-[860px]">
      <div className="dh">
        <div className="flex min-w-0 items-center gap-3">
          <span className="ss-at"><AgentIcon target={target} size={17} /></span>
          <div className="flex min-w-0 flex-col gap-0.5">
            <h2 className="ss-h2 font-mono">{target}</h2>
            <p className="truncate font-mono text-[12.5px] text-ink-2" title={path}>{shortenHome(path)}</p>
          </div>
        </div>
        <button type="button" className="ss-ib" aria-label={t('common.close')} onClick={onClose}><X size={16} /></button>
      </div>
      <div className="db">
        {error ? (
          <div className="ss-note bad"><span className="flex-1">{instructionsErrorMessage(error, t)}</span></div>
        ) : (
          <div className="ss-code flex max-h-[60vh] min-h-[200px] flex-col !overflow-hidden !p-0 !whitespace-normal">
            <BoxHeader content={isPending ? undefined : content} view={view} onChange={setView}
              views={[{ value: 'preview', label: t('instructions.target.view.preview') }, { value: 'source', label: t('instructions.target.view.source') }]} />
            {isPending ? (
              <div className="flex flex-1 items-center justify-center"><Spinner /></div>
            ) : view === 'source' ? (
              <CodeView content={content} lang="markdown" className="min-h-0 flex-1 overflow-auto !rounded-none !border-0" />
            ) : (
              <InstructionsPreview content={content} names={names} />
            )}
          </div>
        )}
      </div>
      <div className="df">
        <span className="flex-1" />
        <Button variant="ghost" onClick={onClose}>{t('common.close')}</Button>
        <Link to={`/targets/${encodeURIComponent(target)}?tab=instructions`} className="ss-btn">{t('instructions.peek.open')}</Link>
      </div>
    </DialogShell>
  );
}
