import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { SelfUsageError } from '../api';

export function PageError({ error, retry }: { error: Error; retry?: () => void }) {
  const { t } = useTranslation();
  const message =
    error instanceof SelfUsageError && error.status === 429
      ? t('selfUsage.error.rateLimit')
      : error instanceof SelfUsageError && error.status === 403
        ? t('selfUsage.error.forbidden')
        : error instanceof SelfUsageError && error.code === 'BAD_USER_INPUT'
          ? error.message
          : t('selfUsage.error.generic');
  return (
    <div role='alert' className='border-destructive/40 text-destructive space-y-2 rounded-md border p-4 text-sm'>
      <p>{message}</p>
      {retry && (
        <Button variant='outline' size='sm' onClick={retry}>
          {t('selfUsage.retry')}
        </Button>
      )}
    </div>
  );
}
