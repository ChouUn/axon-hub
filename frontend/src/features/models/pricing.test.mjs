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
const { multiplyPriceItems, createPriceValidationSchema } = await import(
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

test('an unconfigured price stays absent while an explicitly free price remains configured', () => {
  assert.equal(priceSchema.parse({ items: [], volumeTiers: [] }), null);
  const free = { items: [item('prompt_tokens', '0')], volumeTiers: [] };
  assert.deepEqual(priceSchema.parse(free), free);
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
  assert.deepEqual(priceSchema.parse(price), price);
  price.schedule.overrides[0].items[0].pricing.usagePerUnit = '';
  assert.equal(priceSchema.safeParse(price).success, false);
  price.schedule.overrides = [];
  assert.equal(priceSchema.safeParse(price).success, false);
});
