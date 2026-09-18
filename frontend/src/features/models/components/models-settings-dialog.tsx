'use client';

import React, { useCallback } from 'react';
import { Loader2, Settings2, ListTree, BrainCircuit, EyeOff } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Switch } from '@/components/ui/switch';
import { useModelSettings, useUpdateModelSettings, type UpdateModelSettingsInput } from '@/features/system/data/system';
import { useModels } from '../context/models-context';

export function ModelSettingsDialog() {
  const { t } = useTranslation();
  const { open, setOpen } = useModels();
  const { data: settings, isLoading } = useModelSettings();
  const updateModelSettings = useUpdateModelSettings();

  const isOpen = open === 'settings';

  const [defaultModelAPIIncludeAll, setDefaultModelAPIIncludeAll] = React.useState(false);
  const [autoReasoningEffort, setAutoReasoningEffort] = React.useState(false);
  const [hideUnroutableModelsInList, setHideUnroutableModelsInList] = React.useState(false);

  React.useEffect(() => {
    if (settings) {
      setDefaultModelAPIIncludeAll(settings.defaultModelAPIIncludeAll);
      setAutoReasoningEffort(settings.autoReasoningEffort);
      setHideUnroutableModelsInList(settings.hideUnroutableModelsInList);
    }
  }, [settings]);

  const handleSave = useCallback(async () => {
    const input: UpdateModelSettingsInput = {
      defaultModelAPIIncludeAll: defaultModelAPIIncludeAll,
      autoReasoningEffort: autoReasoningEffort,
      hideUnroutableModelsInList: hideUnroutableModelsInList,
      developerSettings: settings?.developerSettings || [],
    };
    await updateModelSettings.mutateAsync(input);
    setOpen(null);
  }, [
    updateModelSettings,
    defaultModelAPIIncludeAll,
    autoReasoningEffort,
    hideUnroutableModelsInList,
    settings?.developerSettings,
    setOpen,
  ]);

  const handleClose = useCallback(() => {
    setOpen(null);
  }, [setOpen]);

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className='flex max-h-[90vh] w-full max-w-full flex-col overflow-hidden sm:max-w-[720px]'>
        <DialogHeader className='shrink-0'>
          <DialogTitle className='flex items-center gap-2 text-lg sm:text-xl'>
            <Settings2 className='h-5 w-5' />
            {t('models.dialogs.settings.title')}
          </DialogTitle>
          <DialogDescription className='text-sm sm:text-base'>{t('models.dialogs.settings.description')}</DialogDescription>
        </DialogHeader>

        {isLoading ? (
          <div className='flex items-center justify-center py-12'>
            <Loader2 className='h-8 w-8 animate-spin' />
          </div>
        ) : (
          <div className='min-h-0 flex-1 space-y-4 overflow-y-auto pr-1'>
            <Card>
              <CardHeader className='pb-0'>
                <CardTitle className='flex items-center gap-2 text-sm sm:text-base'>
                  <ListTree className='text-muted-foreground h-4 w-4' />
                  {t('models.dialogs.settings.defaultModelAPIIncludeAll.label')}
                </CardTitle>
              </CardHeader>
              <CardContent className='pt-1'>
                <div className='flex items-center justify-between'>
                  <p className='text-muted-foreground pr-4 text-sm'>{t('models.dialogs.settings.defaultModelAPIIncludeAll.description')}</p>
                  <Switch
                    id='default-model-api-include-all'
                    checked={defaultModelAPIIncludeAll}
                    onCheckedChange={setDefaultModelAPIIncludeAll}
                    disabled={updateModelSettings.isPending}
                    className='scale-100 sm:scale-75'
                  />
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className='pb-0'>
                <CardTitle className='flex items-center gap-2 text-sm sm:text-base'>
                  <EyeOff className='text-muted-foreground h-4 w-4' />
                  {t('models.dialogs.settings.hideUnroutableModelsInList.label')}
                </CardTitle>
              </CardHeader>
              <CardContent className='pt-1'>
                <div className='flex items-center justify-between'>
                  <p className='text-muted-foreground pr-4 text-sm'>
                    {t('models.dialogs.settings.hideUnroutableModelsInList.description')}
                  </p>
                  <Switch
                    id='hide-unroutable-models-in-list'
                    checked={hideUnroutableModelsInList}
                    onCheckedChange={setHideUnroutableModelsInList}
                    disabled={updateModelSettings.isPending}
                    className='scale-100 sm:scale-75'
                  />
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className='pb-0'>
                <CardTitle className='flex items-center gap-2 text-sm sm:text-base'>
                  <BrainCircuit className='text-muted-foreground h-4 w-4' />
                  {t('models.dialogs.settings.autoReasoningEffort.label')}
                </CardTitle>
              </CardHeader>
              <CardContent className='pt-1'>
                <div className='flex items-center justify-between'>
                  <p className='text-muted-foreground pr-4 text-sm'>{t('models.dialogs.settings.autoReasoningEffort.description')}</p>
                  <Switch
                    id='auto-reasoning-effort'
                    checked={autoReasoningEffort}
                    onCheckedChange={setAutoReasoningEffort}
                    disabled={updateModelSettings.isPending}
                    className='scale-100 sm:scale-75'
                  />
                </div>
              </CardContent>
            </Card>
          </div>
        )}

        <DialogFooter className='flex shrink-0 flex-col items-stretch justify-between gap-3 sm:flex-row sm:items-center sm:gap-2'>
          <Button variant='outline' onClick={handleClose} disabled={updateModelSettings.isPending} className='h-10 w-full sm:h-9 sm:w-auto'>
            {t('common.buttons.cancel')}
          </Button>
          <Button onClick={handleSave} disabled={updateModelSettings.isPending || isLoading} className='h-10 w-full sm:h-9 sm:w-auto'>
            {updateModelSettings.isPending ? (
              <>
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                {t('common.buttons.saving')}
              </>
            ) : (
              t('common.buttons.save')
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
