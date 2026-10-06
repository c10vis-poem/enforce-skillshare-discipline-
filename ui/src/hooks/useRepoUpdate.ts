import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { clearAuditCache } from '../lib/auditCache';
import { useToast } from '../components/Toast';
import { useT } from '../i18n';
import { invalidate } from '../lib/queryEvents';

/** Pulls one tracked repo and reports the outcome as a toast. Used by the Dashboard and the Skills page. */
export function useRepoUpdate() {
  const t = useT();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [updating, setUpdating] = useState<string | null>(null);

  const update = async (repo: string) => {
    setUpdating(repo);
    const name = repo.replace(/^_/, '');
    try {
      const item = (await api.update({ name: repo })).results[0];
      if (item?.action === 'updated') toast(t('dashboard.toast.repoUpdated', { name, message: item.message ?? 'done' }), 'success');
      else if (item?.action === 'up-to-date') toast(t('dashboard.toast.repoAlreadyUpToDate', { name }), 'info');
      else if (item?.action === 'blocked') toast(item.message ?? t('dashboard.toast.updateBlockedFor', { name }), 'error');
      else if (item?.action === 'error') toast(item.message ?? t('dashboard.toast.updateFailedFor', { name }), 'error');
      else toast(item?.message ?? t('dashboard.toast.repoSkipped', { name }), 'warning');
      clearAuditCache(queryClient);
      await invalidate(queryClient, 'reposChanged');
    } catch (e: unknown) {
      toast((e as Error).message, 'error');
    } finally {
      setUpdating(null);
    }
  };

  return { updating, update };
}
