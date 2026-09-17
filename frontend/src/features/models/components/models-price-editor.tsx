import { useState, type ComponentProps } from 'react';
import { useFieldArray, useFormContext, useWatch } from 'react-hook-form';
import { IconPlus, IconTrash } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { ModelPriceEditor } from '@/components/model-price-editor';
import { PriceScheduleEditor } from '@/components/price-schedule-editor';
import type { ModelPrice, ModelPriceItem } from '@/features/channels/data/schema';
import { useGeneralSettings } from '@/features/system/data/system';
import { multiplyPriceItems } from '../data/pricing';
import type { CreateModelInput } from '../data/schema';

export function ModelsPriceEditor({ portalContainer }: { portalContainer: HTMLDivElement | null }) {
  const { t } = useTranslation();
  const { control } = useFormContext<CreateModelInput>();
  const price = useWatch({ control, name: 'modelCard.price' });
  return (
    <section className='space-y-4 rounded-md border p-4'>
      <h4 className='text-sm font-medium'>{t('models.price.title')}</h4>
      <p className='text-muted-foreground text-xs'>{t('models.price.description')}</p>
      <PriceContent price={{ ...price, items: price?.items ?? [] }} portalContainer={portalContainer} />
    </section>
  );
}

function PriceContent({ price, portalContainer }: { price: ModelPrice; portalContainer: HTMLDivElement | null }) {
  const { t } = useTranslation();
  const { data: settings } = useGeneralSettings();
  const {
    control,
    getValues,
    formState: { errors },
  } = useFormContext<CreateModelInput>();
  const { fields, append, remove, replace } = useFieldArray({ control, name: 'modelCard.price.volumeTiers' });
  const unified = fields.length > 0;
  const canUseUnified =
    price.items.length > 0 &&
    price.items.every(
      (item) =>
        item.pricing.mode === 'usage_per_unit' &&
        (item.promptWriteCacheVariants ?? []).every((variant) => variant.pricing.mode === 'usage_per_unit')
    );
  // The shared editor accepts a dynamic items path; its field shape is the same at this model path.
  const itemControl = control as unknown as ComponentProps<typeof ModelPriceEditor>['control'];
  const errorMessages: string[] = [];
  const collectErrors = (error: unknown) => {
    if (!error || typeof error !== 'object') return;
    if ('message' in error && typeof error.message === 'string') errorMessages.push(error.message);
    Object.entries(error).forEach(([key, value]) => {
      if (key !== 'ref' && key !== 'message' && key !== 'type') collectErrors(value);
    });
  };
  collectErrors(errors.modelCard?.price);
  return (
    <div className='space-y-4'>
      <div className='flex items-center gap-2'>
        <Switch
          id='model-price-unified'
          checked={unified}
          disabled={!unified && !canUseUnified}
          onCheckedChange={(enabled) => {
            if (enabled) append({ above: NaN, items: structuredClone(getValues('modelCard.price.items') ?? []) });
            else replace([]);
          }}
        />
        <Label htmlFor='model-price-unified'>{t('price.volume.title')}</Label>
      </div>
      <p className='text-muted-foreground text-xs'>{t(unified ? 'price.volume.description' : 'price.volume.enableHint')}</p>
      {!canUseUnified && !unified && <p className='text-muted-foreground text-xs'>{t('price.validation.perUnitOnly')}</p>}
      <div className='space-y-3 rounded-md border p-3'>
        <h4 className='text-sm font-medium'>
          {unified && Number.isFinite(price.volumeTiers?.[0]?.above)
            ? t('price.volume.baseRange', { above: price.volumeTiers?.[0]?.above })
            : t('price.volume.base')}
        </h4>
        <ModelPriceEditor
          control={itemControl}
          priceIndex={0}
          itemsPath='modelCard.price.items'
          currencyCode={settings?.currencyCode}
          perUnitOnly={unified}
          hideHeader
        />
      </div>
      {fields.map((field, index) => (
        <VolumeTierRow
          key={field.id}
          index={index}
          above={price.volumeTiers?.[index]?.above ?? NaN}
          nextAbove={price.volumeTiers?.[index + 1]?.above}
          baseItems={price.items}
          currencyCode={settings?.currencyCode}
          onRemove={() => remove(index)}
        />
      ))}
      {unified && (
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() =>
            append({
              above: NaN,
              items: structuredClone(getValues('modelCard.price.items') ?? []),
            })
          }
        >
          <IconPlus size={14} />
          {t('price.volume.addTier')}
        </Button>
      )}
      <PriceScheduleEditor
        control={control as unknown as ComponentProps<typeof PriceScheduleEditor>['control']}
        priceIndex={0}
        pricePath='modelCard.price'
        currencyCode={settings?.currencyCode}
        defaultTimezone={settings?.timezone}
        portalContainer={portalContainer}
        renderItems={(itemsPath) => (
          <ModelPriceEditor control={itemControl} priceIndex={0} itemsPath={itemsPath} currencyCode={settings?.currencyCode} />
        )}
      />
      {price.schedule != null && <p className='text-muted-foreground text-xs'>{t('price.volume.schedulePrecedence')}</p>}
      {errorMessages.length > 0 && (
        <p role='alert' className='text-destructive text-sm'>
          {[...new Set(errorMessages)].join(' · ')}
        </p>
      )}
    </div>
  );
}

function VolumeTierRow({
  index,
  above,
  nextAbove,
  baseItems,
  currencyCode,
  onRemove,
}: {
  index: number;
  above: number;
  nextAbove?: number;
  baseItems: ModelPriceItem[];
  currencyCode?: string;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const { control, setValue, getValues } = useFormContext<CreateModelInput>();
  const [multiplier, setMultiplier] = useState('1');
  const validMultiplier = multiplier.trim() !== '' && Number.isFinite(Number(multiplier)) && Number(multiplier) >= 0;
  const completeBase =
    baseItems.length > 0 &&
    baseItems.every((item) =>
      [item.pricing, ...(item.promptWriteCacheVariants ?? []).map((variant) => variant.pricing)].every(
        (pricing) =>
          pricing.usagePerUnit != null &&
          String(pricing.usagePerUnit).trim() !== '' &&
          Number.isFinite(Number(pricing.usagePerUnit)) &&
          Number(pricing.usagePerUnit) >= 0
      )
    );
  return (
    <div className='space-y-3 rounded-md border p-3'>
      <div className='flex items-center justify-between gap-2'>
        <h4 className='text-sm font-medium'>
          {Number.isFinite(above)
            ? t(Number.isFinite(nextAbove) ? 'price.volume.betweenRange' : 'price.volume.aboveRange', { above, upTo: nextAbove })
            : t('price.volume.enterThreshold')}
        </h4>
        <Button type='button' variant='ghost' size='icon-sm' aria-label={t('price.volume.removeTier')} onClick={onRemove}>
          <IconTrash size={14} />
        </Button>
      </div>
      <div className='flex flex-wrap items-end gap-3'>
        <FormField
          control={control}
          name={`modelCard.price.volumeTiers.${index}.above`}
          render={({ field }) => (
            <FormItem className='min-w-40 flex-1'>
              <FormLabel>{t('price.volume.above')}</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  type='number'
                  min={0}
                  step={1}
                  placeholder={t('price.volume.enterThreshold')}
                  value={Number.isFinite(field.value) ? field.value : ''}
                  onChange={(event) => field.onChange(event.target.value === '' ? NaN : Number(event.target.value))}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <div className='space-y-2'>
          <Label htmlFor={`tier-multiplier-${index}`}>{t('price.volume.multiplier')}</Label>
          <Input
            id={`tier-multiplier-${index}`}
            className='w-28'
            type='number'
            min={0}
            step='any'
            value={multiplier}
            onChange={(event) => setMultiplier(event.target.value)}
          />
        </div>
        <Button
          type='button'
          variant='outline'
          disabled={!validMultiplier || !completeBase}
          onClick={() =>
            setValue(
              `modelCard.price.volumeTiers.${index}.items`,
              multiplyPriceItems(getValues('modelCard.price.items') ?? [], multiplier),
              {
                shouldDirty: true,
                shouldValidate: true,
              }
            )
          }
        >
          {t('price.volume.fill')}
        </Button>
      </div>
      <p className='text-muted-foreground text-xs'>{t('price.volume.multiplierHint')}</p>
      <ModelPriceEditor
        control={control as unknown as ComponentProps<typeof ModelPriceEditor>['control']}
        priceIndex={0}
        itemsPath={`modelCard.price.volumeTiers.${index}.items`}
        currencyCode={currencyCode}
        perUnitOnly
        hideHeader
      />
    </div>
  );
}
