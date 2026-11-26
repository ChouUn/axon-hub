import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';

function moduleURL(path, imports = {}) {
  const source = ts.transpileModule(readFileSync(new URL(path, import.meta.url), 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2023 },
  }).outputText;
  const resolved = source.replace(
    /(from\s+['"])([^'"]+)(['"])/g,
    (_, prefix, name, suffix) => `${prefix}${imports[name] ?? import.meta.resolve(name)}${suffix}`
  );
  return `data:text/javascript;base64,${Buffer.from(resolved).toString('base64')}`;
}

const paginationURL = moduleURL('../../gql/pagination.ts');
const schemaURL = moduleURL('../channels/data/schema.ts', { '@/gql/pagination': paginationURL });
const { modelPriceInputSchema } = await import(schemaURL);
const { multiplyPriceItems, createPriceValidationSchema, priceFromCatalog } = await import(
  moduleURL('./data/pricing.ts', { '@/features/channels/data/schema': schemaURL })
);
const priceSchema = createPriceValidationSchema((key) => key);
const item = (itemCode, amount) => ({ itemCode, pricing: { mode: 'usage_per_unit', usagePerUnit: amount } });

test('bulk tier filling preserves decimal prices and cache variants without mutating the base', () => {
  const base = [
    item('prompt_tokens', '0.1'),
    item('completion_tokens', '1e-7'),
    {
      ...item('prompt_write_cached_tokens', '0'),
      promptWriteCacheVariants: [{ variantCode: 'one_hour', pricing: item('', '0.123456789012345678901').pricing }],
    },
  ];
  const original = structuredClone(base);
  const filled = multiplyPriceItems(base, '3');
  assert.equal(filled[0].pricing.usagePerUnit, '0.3');
  assert.equal(filled[1].pricing.usagePerUnit, '0.0000003');
  assert.equal(filled[2].pricing.usagePerUnit, '0');
  assert.equal(filled[2].promptWriteCacheVariants[0].pricing.usagePerUnit, '0.370370367037037036703');
  assert.deepEqual(base, original);
  assert.equal(
    multiplyPriceItems([item('prompt_tokens', '0.1')], '1.00000000000000000001')[0].pricing.usagePerUnit,
    '0.100000000000000000001'
  );
});

test('anthropic catalog imports bill one-hour cache writes at twice the input price', () => {
  const oneHour = (items) =>
    items.find((entry) => entry.itemCode === 'prompt_write_cached_tokens').promptWriteCacheVariants?.map(({ variantCode, pricing }) => [variantCode, pricing.usagePerUnit]);
  const model = {
    cost: { input: 0.3, output: 15, cache_read: 0.03, cache_write: 0.375, tiers: [{ input: 6, cache_write: 7.5, tier: { type: 'context', size: 200000 } }] },
  };
  const price = priceFromCatalog(model, 'anthropic');
  assert.deepEqual(oneHour(price.items), [['one_hour', '0.6']]);
  assert.equal(price.items.find((entry) => entry.itemCode === 'prompt_write_cached_tokens').pricing.usagePerUnit, '0.375');
  assert.deepEqual(oneHour(price.volumeTiers[0].items), [['one_hour', '12']]);
  assert.equal(priceSchema.safeParse(price).success, true);

  assert.equal(oneHour(priceFromCatalog(model, 'amazon-bedrock').items), undefined);
  const noWrite = priceFromCatalog({ cost: { input: 3, output: 15 } }, 'anthropic');
  assert.equal(noWrite.items.some((entry) => entry.promptWriteCacheVariants), false);
});

test('an unconfigured price stays absent while an explicitly free price remains configured', () => {
  assert.equal(priceSchema.parse({ items: [], volumeTiers: [] }), null);
  const free = { items: [item('prompt_tokens', '0')], volumeTiers: [] };
  assert.equal(priceSchema.parse(free).items[0].pricing.usagePerUnit, '0');
});

test('unfinished items and volume tiers cannot be mistaken for an unconfigured price', () => {
  assert.equal(priceSchema.safeParse({ items: [item('prompt_tokens', '')] }).success, false);
  assert.equal(priceSchema.safeParse({ items: [], volumeTiers: [{ above: 272000, items: [] }] }).success, false);
});

test('schedule-only pricing can be edited but incomplete override prices are rejected', () => {
  const price = {
    items: [],
    schedule: {
      timezone: 'UTC',
      overrides: [
        { name: 'night', priority: 1, when: { dailyTime: { start: '01:00', end: '02:00' } }, items: [item('prompt_tokens', '0.5')] },
      ],
    },
  };
  price.schedule.overrides[0].items[0].pricing.usagePerUnit = '';
  assert.equal(priceSchema.safeParse(price).success, false);
  price.schedule.overrides = [];
  assert.equal(priceSchema.safeParse(price).success, false);
});

const variantOnlyCacheWrite = {
  itemCode: 'prompt_write_cached_tokens',
  pricing: { mode: '', flatFee: '1', usagePerUnit: '2', usageTiered: { tiers: [{ upTo: null, pricePerUnit: '3' }] } },
  promptWriteCacheVariants: [
    { variantCode: 'five_min', pricing: item('', '1.25').pricing },
    { variantCode: 'one_hour', pricing: item('', '2').pricing },
  ],
};

test('variant-only cache write pricing round-trips with a null aggregate mode', () => {
  const price = { items: [variantOnlyCacheWrite] };
  const input = modelPriceInputSchema.parse(price);
  assert.deepEqual(input.items[0].pricing, { mode: null, flatFee: null, usagePerUnit: null, usageTiered: null });
  assert.deepEqual(input.items[0].promptWriteCacheVariants.map(({ pricing }) => pricing.usagePerUnit), ['1.25', '2']);
  assert.deepEqual(priceSchema.parse(price), input);
  assert.deepEqual(priceSchema.parse(input), input);
});

test('other item codes and cache write variants require a pricing mode', () => {
  const otherItem = { ...variantOnlyCacheWrite, itemCode: 'prompt_tokens' };
  assert.equal(modelPriceInputSchema.safeParse({ items: [otherItem] }).success, false);
  assert.equal(priceSchema.safeParse({ items: [otherItem] }).success, false);
  assert.equal(modelPriceInputSchema.safeParse({ items: [{ ...variantOnlyCacheWrite, promptWriteCacheVariants: [] }] }).success, false);
  const invalidVariant = { variantCode: 'five_min', pricing: { mode: null } };
  assert.equal(
    modelPriceInputSchema.safeParse({ items: [{ ...variantOnlyCacheWrite, promptWriteCacheVariants: [invalidVariant] }] }).success,
    false
  );
});

test('volume tier items require per-unit pricing even when cache write variants have per-unit prices', () => {
  const baseItem = { ...variantOnlyCacheWrite, pricing: item('', '1.25').pricing };
  const price = { items: [baseItem], volumeTiers: [{ above: 200000, items: [variantOnlyCacheWrite] }] };
  const result = priceSchema.safeParse(price);
  assert.equal(result.success, false);
  assert.deepEqual(result.error.issues.map(({ path, message }) => ({ path, message })), [
    { path: ['volumeTiers', 0, 'items', 0, 'pricing', 'mode'], message: 'price.validation.perUnitOnly' },
  ]);
});
