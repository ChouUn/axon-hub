// 职责: 对比模型当前价格与目录来源价格，并构造整模型的导入选择。
// 关联文件:
//   frontend/src/features/models/components/models-catalog-import.tsx
//   frontend/src/features/models/components/models-price-display.tsx
//   frontend/src/features/models/data/catalog-import.ts
//   frontend/src/features/models/data/schema.ts
//   frontend/src/locales/en/models.json
//   frontend/src/locales/zh-CN/models.json
// 注意事项:
//   未绑定来源的历史价格默认保留，采用目录价格时同时应用其中的阶梯和缓存报价。
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import type { ImportRow, Offer, PriceBinding, PriceSelection, ImportSelection } from '../data/catalog-import';
import { modelTypeSchema } from '../data/schema';
import { ModelsPriceDisplay } from './models-price-display';

export const selectStyle = 'h-9 max-w-full rounded-md border bg-background px-2 text-sm';
export const offerKey = (offer: { provider: string; modelID: string }) => JSON.stringify([offer.provider, offer.modelID]);

export const priceActionLabels = {
  FOLLOW: 'models.import.follow',
  REBIND: 'models.import.rebind',
  FIX_SOURCE: 'models.import.fixBase',
  FIX: 'models.import.fixCurrent',
} as const;

function findOffer(row: ImportRow, source: Pick<Offer, 'provider' | 'modelID'> | null) {
  return row.offers.find((offer) => offer.provider === source?.provider && offer.modelID === source.modelID);
}

function needsRebind(binding: PriceBinding | null, offer: Offer) {
  return binding?.follow && (binding.provider !== offer.provider || binding.modelID !== offer.modelID || binding.canonical !== offer.canonical);
}

function sourcePriceSelection(offer: Offer, action: Exclude<PriceSelection['action'], 'FIX'>): PriceSelection {
  return { action, provider: offer.provider, modelID: offer.modelID, identity: offer.identity, expectedDigest: offer.priceDigest };
}

function defaultRequest(modelID: string, route: ImportRow['routes'][number]) {
  if (route.requests.includes(modelID)) return modelID;
  if (route.requests.includes(route.actual)) return route.actual;
  if (!route.requests.length) return '';
  return route.requests.length === 1 ? route.requests[0] : undefined;
}

export function defaultSelection(
  row: ImportRow,
  offer = findOffer(row, row.binding) ?? findOffer(row, row.preferred) ?? null
): ImportSelection {
  const follow = offer?.price && (!row.version || (row.changed && row.binding?.follow && !needsRebind(row.binding, offer)));
  return {
    type: offer?.type ? (modelTypeSchema.safeParse(offer.type).data ?? '') : 'chat',
    identity: offer?.identity ?? '',
    modelID: row.modelID,
    members: row.members,
    manualMerge: row.manualMerge,
    routes: row.version
      ? []
      : row.routes.map((route) => ({ channelID: route.channelID, actual: route.actual, request: defaultRequest(row.modelID, route) })),
    version: row.version,
    name: row.version ? row.name : offer?.name || row.modelID,
    developer: row.version ? '' : offer?.developer || '',
    provider: offer?.provider || '',
    sourceModelID: offer?.modelID || '',
    price: follow && offer ? sourcePriceSelection(offer, 'FOLLOW') : null,
  };
}

export function PriceEditor({
  row,
  draft,
  offer,
  onChange,
}: {
  row: ImportRow;
  draft: ImportSelection;
  offer: Offer | undefined;
  onChange: (value: ImportSelection) => void;
}) {
  const { t } = useTranslation();
  const boundOffer = findOffer(row, row.binding);
  const followAction = offer && needsRebind(row.binding, offer) ? 'REBIND' : 'FOLLOW';

  function selectPriceAction(action: string) {
    let price: PriceSelection | null = null;
    if (action === 'FIX' && row.version && row.binding?.follow) {
      price = { action, provider: '', modelID: '', identity: '', expectedDigest: '' };
    } else if (offer?.price && (action === followAction || action === 'FIX_SOURCE')) {
      price = sourcePriceSelection(offer, action);
    }
    onChange({ ...draft, price });
  }

  function selectOffer(next: Offer | null) {
    const defaults = defaultSelection(row, next);
    const previous = defaultSelection(row, offer ?? null);
    for (const field of ['name', 'developer', 'type'] as const) {
      if (draft[field] !== previous[field]) defaults[field] = draft[field];
    }
    onChange({ ...defaults, routes: draft.routes });
  }
  return (
    <div className='space-y-3 rounded-md border p-3'>
      {!row.version && (
        <div className='grid gap-2 sm:grid-cols-3'>
          <label className='text-sm'>
            {t('models.fields.name')}
            <Input value={draft.name} onChange={(e) => onChange({ ...draft, name: e.target.value })} />
          </label>
          <label className='text-sm'>
            {t('models.fields.developer')}
            <Input value={draft.developer} onChange={(e) => onChange({ ...draft, developer: e.target.value })} />
          </label>
          <label className='grid text-sm'>
            {t('models.fields.type')}
            <select className={selectStyle} value={draft.type} onChange={(e) => onChange({ ...draft, type: e.target.value })}>
              {!draft.type && <option value=''>{t('models.import.pickType')}</option>}
              {modelTypeSchema.options.map((type) => (
                <option key={type} value={type}>
                  {t('models.types.' + type)}
                </option>
              ))}
            </select>
          </label>
        </div>
      )}
      {row.members.length > 1 && (
        <p className='text-xs break-words'>
          {t('models.import.aliases')}: {row.members.map((m) => m.id).join(', ')}
        </p>
      )}
      {!row.version &&
        row.routes.map((route, index) => {
          const key = route.channelID + '/' + route.actual;
          const label = t('models.import.route', { channel: route.channelName, model: route.actual });
          if (defaultRequest(row.modelID, route))
            return (
              <p key={key} className='text-muted-foreground text-xs break-all'>
                {label} · {draft.routes[index].request}
              </p>
            );
          if (!route.requests.length)
            return (
              <p key={key} className='text-muted-foreground text-xs'>
                {t('models.import.hiddenRoute', { channel: route.channelName, model: route.actual })}
              </p>
            );
          return (
            <label key={key} className='grid gap-1 text-sm'>
              {label}
              <select
                className={selectStyle}
                value={draft.routes[index].request ?? ''}
                onChange={(e) =>
                  onChange({
                    ...draft,
                    routes: draft.routes.map((r, i) => (i === index ? { ...r, request: e.target.value || undefined } : r)),
                  })
                }
              >
                <option value=''>{t('models.import.pickRequest')}</option>
                {route.requests.map((name) => (
                  <option key={name}>{name}</option>
                ))}
              </select>
            </label>
          );
        })}
      {row.offers.length > 0 && (
        <label className='flex flex-wrap items-center gap-2 text-sm'>
          {t('models.import.priceSource')}
          <select
            className={selectStyle}
            value={offer ? offerKey(offer) : ''}
            onChange={(e) => selectOffer(row.offers.find((o) => offerKey(o) === e.target.value) ?? null)}
          >
            <option value=''>{t('models.import.pickSource')}</option>
            {row.offers.map((o) => (
              <option key={offerKey(o)} value={offerKey(o)}>
                {o.provider} · {o.modelID}
              </option>
            ))}
          </select>
        </label>
      )}
      {!row.version && draft.price && (
        <Button type='button' size='sm' variant='outline' data-testid='catalog-details-only' onClick={() => onChange({ ...draft, price: null })}>
          {t('models.import.noSource')}
        </Button>
      )}
      <div className='grid min-w-0 gap-3 border-t pt-3' data-testid='catalog-price-row'>
        <div className='grid min-w-0 gap-3 sm:grid-cols-2'>
          <div className='min-w-0 space-y-1 break-words'>
            <h4 className='text-muted-foreground text-xs'>{t('models.import.currentPrice')}</h4>
            <ModelsPriceDisplay price={row.price} />
            {(row.price || row.binding) && (
              <p className='text-muted-foreground text-xs'>
                {row.binding?.follow
                  ? t('models.import.following', { provider: row.binding.provider, model: row.binding.modelID })
                  : t('models.import.manual')}
              </p>
            )}
            {row.binding?.follow && boundOffer && row.binding.canonical !== boundOffer.canonical && (
              <p className='text-xs'>
                {t('models.import.identityChanged', { from: row.binding.canonical || '—', to: boundOffer.canonical || '—' })}
              </p>
            )}
            {row.binding?.follow && !boundOffer?.price && (
              <p className='text-muted-foreground text-xs'>{t('models.import.sourceMissingPrice')}</p>
            )}
          </div>
          <div className='min-w-0 space-y-1 break-words'>
            <h4 className='text-muted-foreground text-xs'>{t('models.import.referencePrice')}</h4>
            {offer && !offer.price ? (
              <p className='text-muted-foreground text-xs'>{t('models.import.noQuotes')}</p>
            ) : (
              <ModelsPriceDisplay price={offer?.price} />
            )}
          </div>
        </div>
        <label className='grid gap-1 text-sm sm:justify-self-start'>
          {t('models.import.onApply')}
          <select
            data-testid='catalog-price-action'
            className={selectStyle}
            value={draft.price?.action ?? ''}
            onChange={(e) => selectPriceAction(e.target.value)}
          >
            <option value=''>{t('models.import.keep')}</option>
            <option value={followAction} disabled={!offer?.price}>
              {t(priceActionLabels[followAction])}
            </option>
            <option value='FIX_SOURCE' disabled={!offer?.price}>
              {t(priceActionLabels.FIX_SOURCE)}
            </option>
            {row.version && row.binding?.follow && <option value='FIX'>{t(priceActionLabels.FIX)}</option>}
          </select>
        </label>
      </div>
    </div>
  );
}
