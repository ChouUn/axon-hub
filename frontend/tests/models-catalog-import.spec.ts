// 职责: 验证批量导入的整模型价格选择、来源绑定和刷新后的跨页草稿。
// 关联文件:
//   frontend/src/features/models/components/models-catalog-import.tsx
//   frontend/src/features/models/components/models-catalog-prices.tsx
//   frontend/src/features/models/data/catalog-import.ts
// 注意事项:
//   导入提交使用隔离响应。
import { expect, test } from '@playwright/test';
import type { ImportRow, ImportSelection, Offer } from '../src/features/models/data/catalog-import';
import { gotoAndEnsureAuth } from './auth.utils';

test.beforeEach(async ({ page }) => {
  await page.route('**/admin/graphql', async (route) => {
    if (route.request().postDataJSON().operationName !== 'OnboardingInfo') return route.fallback();
    const onboardingInfo = { onboarded: true, systemModelSetting: { onboarded: true }, autoDisableChannel: { onboarded: true } };
    return route.fulfill({ json: { data: { onboardingInfo } } });
  });
});

test('a filtered batch keeps same-source quotes and resets when the catalog source changes', async ({ page }) => {
  let inputPrice = 1;
  let upstreamURL = 'https://a.test/catalog';
  let source = 'upstream';
  let submittedRevision: string | undefined;
  const provider = 'model-test';
  let reads = 0;
  let applied: ImportSelection[] = [];
  await page.route('**/admin/graphql', async (route) => {
    const body = route.request().postDataJSON();
    if (body.operationName === 'ApplyCatalogImport') {
      applied = body.variables.selections;
      submittedRevision = body.variables.revision;
      const outcomes = body.variables.selections.map(({ modelID }: { modelID: string }) => ({ modelID, status: 'catalogChanged' }));
      return route.fulfill({ json: { data: { applyCatalogImport: outcomes } } });
    }
    if (body.operationName !== 'CatalogImportPreview') return route.fallback();
    reads++;
    const rows = Array.from({ length: 28 }, (_, i): ImportRow => {
      const modelID = i < 27 ? 'batch-' + i : 'outside';
      const offer: Offer = {
        provider,
        modelID,
        name: modelID,
        developer: provider,
        identity: 'original',
        canonical: modelID,
        type: 'chat',
        price: {
          items: [
            { itemCode: 'prompt_tokens', pricing: { mode: 'usage_per_unit', usagePerUnit: String(inputPrice) } },
            {
              itemCode: 'prompt_write_cached_tokens',
              pricing: { mode: '' },
              promptWriteCacheVariants: [{ variantCode: 'one_hour', pricing: { mode: 'usage_per_unit', usagePerUnit: '2.5' } }],
            },
          ],
        },
        priceDigest: 'price-' + inputPrice,
      };
      return {
        modelID,
        name: modelID,
        members: [{ id: modelID, identity: 'original' }],
        routes: [],
        version: modelID === 'outside' ? 'saved' : '',
        mergeTarget: '',
        mergeTargetOccupied: false,
        changed: false,
        archived: false,
        offers: [offer],
        preferred: offer,
        price: null,
        binding: null,
      };
    });
    const preview = { providers: [provider], source, upstreamURL, revision: source + upstreamURL, rows };
    return route.fulfill({ json: { data: { catalogImportPreview: preview } } });
  });
  await gotoAndEnsureAuth(page, '/models');
  await page.getByTestId('catalog-import-open').click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('textbox').fill('batch-');
  await expect(dialog.getByText(/Models: 27|共 27 个模型/)).toBeVisible();
  await dialog.getByTestId('catalog-import-select-all').click();
  await expect(dialog.getByTestId('catalog-import-apply')).toContainText('27');
  await dialog.getByTestId('catalog-import-expand-batch-0').click();
  await page.setViewportSize({ width: 390, height: 844 });
  const priceRow = dialog.getByTestId('catalog-price-row');
  const priceAction = priceRow.getByTestId('catalog-price-action');
  await priceAction.scrollIntoViewIfNeeded();
  await expect(priceRow).toContainText(/2[.,]5/);
  const controlBox = await priceAction.boundingBox();
  const listBox = await dialog.getByTestId('catalog-import-rows').boundingBox();
  expect(controlBox!.x).toBeGreaterThanOrEqual(listBox!.x);
  expect(controlBox!.x + controlBox!.width).toBeLessThanOrEqual(listBox!.x + listBox!.width);
  await priceAction.selectOption('FIX_SOURCE');

  await page.clock.install();
  async function refresh() {
    const previousReads = reads;
    await page.clock.fastForward(61_000);
    await page.evaluate(() => window.dispatchEvent(new Event('offline')));
    await page.evaluate(() => window.dispatchEvent(new Event('online')));
    await expect.poll(() => reads).toBeGreaterThan(previousReads);
  }
  inputPrice = 2;
  await refresh();
  await dialog.getByTestId('catalog-import-apply').click();
  await expect.poll(() => applied.length).toBe(27);
  expect(applied.every(({ price }) => price?.expectedDigest === 'price-1')).toBe(true);
  expect(applied[0].price).toEqual({
    action: 'FIX_SOURCE', provider, modelID: 'batch-0', identity: 'original', expectedDigest: 'price-1',
  });
  expect(applied.slice(1).every(({ price }) => price?.action === 'FOLLOW')).toBe(true);

  const firstRow = dialog.getByRole('checkbox', { name: 'batch-0', exact: true });
  for (const nextSource of ['upstream', 'fallback']) {
    await firstRow.check();
    await dialog.getByTestId('catalog-import-expand-batch-0').click();
    await priceAction.selectOption('FIX_SOURCE');
    upstreamURL = 'https://b.test/catalog';
    source = nextSource;
    await refresh();
    await expect(firstRow).not.toBeChecked();
    await expect(dialog.getByText(upstreamURL, { exact: false })).toBeVisible();
    await expect(dialog.getByTestId('catalog-import-apply')).toBeDisabled();
    await expect(priceAction).toHaveCount(0);
  }
  await expect(dialog.getByText(/Using the bundled snapshot|正在使用内置快照/)).toBeVisible();
  await dialog.getByRole('textbox').fill('outside');
  await expect(dialog.getByRole('checkbox', { name: 'outside' })).toBeDisabled();
  await dialog.getByRole('textbox').fill('');
  await firstRow.check();
  await dialog.getByTestId('catalog-import-apply').click();
  await expect.poll(() => submittedRevision).toBe(source + upstreamURL);
  expect(applied[0].price).toEqual({
    action: 'FOLLOW', provider, modelID: 'batch-0', identity: 'original', expectedDigest: 'price-2',
  });
});

test('a model price uses one source and one action, including rebinding and fixing an existing price', async ({ page }) => {
  const details = { name: 'Model', developer: 'dev', identity: 'v1', canonical: 'dev/current', type: 'chat' };
  const inputPrice = (value: string): NonNullable<Offer['price']> => ({
    items: [{ itemCode: 'prompt_tokens', pricing: { mode: 'usage_per_unit', usagePerUnit: value } }],
  });
  const providerA: Offer = { ...details, provider: 'provider-a', modelID: 'mixed', price: inputPrice('1'), priceDigest: 'price-a' };
  const providerB: Offer = {
    ...details, provider: 'provider-b', modelID: 'mixed', priceDigest: 'price-b',
    price: { ...inputPrice('9'), volumeTiers: [{ above: 256000, items: inputPrice('25').items }] },
  };
  const row = (id: string): ImportRow => ({
    name: details.name, modelID: id, members: [{ id, identity: '' }], routes: [], version: '',
    mergeTarget: '', mergeTargetOccupied: false, changed: false, archived: false, offers: [], preferred: null, price: null, binding: null,
  });
  const followed = { provider: 'provider-a', modelID: 'mixed', canonical: 'dev/current', follow: true };
  const free: Offer = {
    ...providerA, modelID: 'zero', name: 'Vector embedding', type: 'embedding', price: inputPrice('0'), priceDigest: 'price-zero',
  };
  const unpriced = { ...free, provider: 'provider-b', price: null, priceDigest: '' };
  const changedOffer = { ...providerA, modelID: 'changed' };
  const alternate = { ...providerB, modelID: 'changed' };
  const previews: ImportRow[] = [
    {
      ...row('mixed'), version: 'saved', offers: [providerA, providerB], preferred: providerB,
      price: inputPrice('3'), binding: { ...followed, canonical: 'dev/previous' },
    },
    {
      ...row('changed'), version: 'saved', changed: true, offers: [changedOffer, alternate], preferred: alternate,
      price: inputPrice('3'), binding: { ...followed, modelID: 'changed' },
    },
    {
      ...row('removed'), version: 'saved', price: inputPrice('3'),
      binding: { ...followed, provider: 'removed', modelID: 'removed' },
    },
    {
      ...row('manual'), version: 'saved', price: inputPrice('3'),
      binding: { ...followed, modelID: 'manual', follow: false },
    },
    {
      ...row('zero'), offers: [free, unpriced], preferred: free,
      routes: [{ channelID: 12, channelName: 'Test channel', actual: 'zero', requests: ['public-zero'] }],
    },
  ];
  let applied: unknown[] = [];
  await page.route('**/admin/graphql', (route) => {
    const body = route.request().postDataJSON();
    if (body.operationName === 'ApplyCatalogImport') {
      applied = body.variables.selections;
      const outcomes = body.variables.selections.map(({ modelID }: { modelID: string }) => ({ modelID, status: 'applied' }));
      return route.fulfill({ json: { data: { applyCatalogImport: outcomes } } });
    }
    if (body.operationName !== 'CatalogImportPreview') return route.fallback();
    const preview = { rows: previews, providers: [], source: 'upstream', upstreamURL: 'https://example.test', revision: 'same' };
    return route.fulfill({ json: { data: { catalogImportPreview: preview } } });
  });
  await gotoAndEnsureAuth(page, '/models');
  await page.getByTestId('catalog-import-open').click();
  const expand = (id: string) => page.getByTestId('catalog-import-expand-' + id);
  await expand('mixed').click();
  const source = page.getByRole('combobox', { name: /Price source|报价来源/ });
  const priceRow = page.getByTestId('catalog-price-row');
  const priceAction = priceRow.getByTestId('catalog-price-action');
  const apply = page.getByTestId('catalog-import-apply');
  await expect(source.locator('option:checked')).toHaveText('provider-a · mixed');
  await expect(priceRow).toContainText('provider-a · mixed');
  await expect(priceRow).toContainText('dev/previous');
  await expect(priceRow).toContainText('dev/current');
  await expect(priceRow).toContainText(/3[.,]0/);
  await expect(priceRow).toContainText(/1[.,]0/);
  await expect(priceAction).toHaveValue('');
  await page.getByRole('checkbox', { name: 'mixed', exact: true }).check();
  await expect(apply).toBeDisabled();
  await source.selectOption({ label: 'provider-b · mixed' });
  await expect(priceRow).toContainText('256000');
  await expect(priceRow).toContainText(/25[.,]0/);
  await expect(priceAction.locator('option[value="REBIND"]')).toContainText(/Confirm source|确认来源/);
  await priceAction.selectOption('REBIND');
  await expect(apply).toBeEnabled();

  await expand('changed').click();
  await expect(source.locator('option:checked')).toHaveText('provider-a · changed');
  await expect(priceAction).toHaveValue('FOLLOW');
  await page.getByRole('checkbox', { name: 'changed', exact: true }).check();

  await expand('removed').click();
  await expect(priceRow).toContainText(/The followed source has no current price|跟随的来源暂无报价/);
  await expect(priceAction.locator('option[value="FOLLOW"]')).toBeDisabled();
  await priceAction.selectOption('FIX');
  await page.getByRole('checkbox', { name: 'removed', exact: true }).check();

  await expand('manual').click();
  await expect(priceRow).toContainText(/Manually maintained|手动维护/);
  await expect(priceAction.locator('option[value="FIX"]')).toHaveCount(0);

  await expand('zero').click();
  await expect(priceRow).toContainText(/0[.,]0/);
  await expect(priceAction).toHaveValue('FOLLOW');
  await expect(page.getByText(/Test channel.*zero.*public-zero|zero.*Test channel.*public-zero/)).toBeVisible();
  await source.selectOption('');
  await expect(expand('zero')).toContainText(/Choose a price source|请选择报价来源/);
  await source.selectOption({ label: 'provider-b · zero' });
  await expect(priceRow).toContainText(/No source price available|来源暂无报价/);
  await expect(priceAction.locator('option[value="FOLLOW"]')).toBeDisabled();
  await expect(priceAction.locator('option[value="FIX_SOURCE"]')).toBeDisabled();
  await source.selectOption({ label: 'provider-a · zero' });
  await page.getByTestId('catalog-details-only').click();
  await expect(priceAction).toHaveValue('');
  await expect(expand('zero')).toContainText(/No source price selected|未采用来源价格/);
  await page.getByRole('checkbox', { name: 'zero', exact: true }).check();
  const developer = page.getByRole('dialog').getByRole('textbox', { name: /Developer|开发者/i });
  await expect(developer).toHaveValue('dev');
  await developer.fill('');
  await expect(page.getByTestId('catalog-import-apply')).toBeDisabled();
  await developer.fill('custom');
  await page.getByTestId('catalog-import-apply').click();
  await expect.poll(() => applied).toEqual([
    expect.objectContaining({
      modelID: 'mixed', developer: '',
      price: { action: 'REBIND', provider: 'provider-b', modelID: 'mixed', identity: 'v1', expectedDigest: 'price-b' },
    }),
    expect.objectContaining({
      modelID: 'changed',
      price: { action: 'FOLLOW', provider: 'provider-a', modelID: 'changed', identity: 'v1', expectedDigest: 'price-a' },
    }),
    expect.objectContaining({
      modelID: 'removed', price: { action: 'FIX', provider: '', modelID: '', identity: '', expectedDigest: '' },
    }),
    expect.objectContaining({
      modelID: 'zero', provider: 'provider-a', sourceModelID: 'zero', identity: 'v1',
      type: 'embedding', name: 'Vector embedding', developer: 'custom', price: null,
      routes: [{ channelID: 12, actual: 'zero', request: 'public-zero' }],
    }),
  ]);
});

test('a lost apply response requires a refreshed preview before retrying', async ({ page }) => {
  let online = true;
  let submissions = 0;
  await page.route('**/admin/graphql', async (route) => {
    const { operationName } = route.request().postDataJSON();
    if (operationName === 'ApplyCatalogImport') {
      submissions++;
      online = false;
      return route.abort();
    }
    if (operationName !== 'CatalogImportPreview') return route.fallback();
    if (!online) return route.fulfill({ json: { errors: [{ message: 'Preview temporarily unavailable' }] } });
    const offer: Offer = { provider: 'dev', modelID: 'item', name: 'Item', developer: 'dev', type: 'chat', canonical: '', identity: '', price: null, priceDigest: '' };
    const row: ImportRow = {
      modelID: offer.modelID,
      name: offer.name,
      version: '',
      members: [{ id: 'item', identity: '' }],
      routes: [],
      offers: [offer],
      preferred: offer,
      price: null,
      binding: null,
      mergeTarget: '',
      mergeTargetOccupied: false,
      archived: false,
      changed: false,
    };
    const preview = { rows: submissions ? [] : [row], providers: ['dev'], source: 'upstream', upstreamURL: 'example', revision: 'mirror' };
    return route.fulfill({ json: { data: { catalogImportPreview: preview } } });
  });
  await gotoAndEnsureAuth(page, '/models');
  await page.getByTestId('catalog-import-open').click();
  await page.getByTestId('catalog-import-select-all').click();
  await page.getByTestId('catalog-import-apply').click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByText(/result could not be confirmed|无法确认提交结果/)).toBeVisible();
  await expect(dialog.getByText('Preview temporarily unavailable', { exact: false })).toBeVisible();
  await expect(dialog.getByTestId('catalog-import-apply')).toHaveCount(0);
  online = true;
  await dialog.getByTestId('catalog-import-reload').click();
  await expect(dialog.getByTestId('catalog-import-apply')).toBeDisabled();
  expect(submissions).toBe(1);
});
