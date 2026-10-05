import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { api } from '../../api/client';
import type { SourceLink } from '../../api/client';
import { useT } from '../../i18n';
import { queryKeys } from '../../lib/queryKeys';
import ConfirmDialog from '../ConfirmDialog';
import { useToast } from '../Toast';

export default function UnlinkFolderDialog({ link, onClose }: { link: SourceLink; onClose: () => void }) {
  const t = useT();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [saving, setSaving] = useState(false);
  const [failure, setFailure] = useState('');

  const unlink = async () => {
    if (saving) return;
    setSaving(true);
    setFailure('');
    try {
      await api.removeSourceLink(link.name);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.skills.all }),
        queryClient.invalidateQueries({ queryKey: queryKeys.overview }),
        queryClient.invalidateQueries({ queryKey: queryKeys.config }),
        queryClient.invalidateQueries({ queryKey: queryKeys.diff() }),
        queryClient.invalidateQueries({ queryKey: queryKeys.syncMatrix() }),
        queryClient.invalidateQueries({ queryKey: queryKeys.trash }),
      ]);
      toast(t('sourceLinks.unlinked', { name: link.name }), 'success');
      onClose();
    } catch (err) {
      setFailure((err as Error).message);
      setSaving(false);
    }
  };

  return (
    <ConfirmDialog open title={t('sourceLinks.unlinkTitle', { name: link.name })} confirmText={t('sourceLinks.unlink')}
      variant="danger" loading={saving} onConfirm={() => void unlink()} onCancel={onClose}
      message={
        <div className="flex flex-col gap-3">
          <p className="break-all font-mono text-xs text-ink-3">{link.target}</p>
          <p>{t('sourceLinks.unlinkMessage')}</p>
          {failure && <div className="ss-note bad" role="alert">{failure}</div>}
        </div>
      }
    />
  );
}
