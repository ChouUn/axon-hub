import assert from 'node:assert/strict';
import test from 'node:test';
import { buildCostBreakdown } from './request-usage-cost-breakdown.ts';

const item = (itemCode, quantity, subtotal, variant = null, tiers = []) => ({ itemCode, variant, quantity, subtotal, tiers });
const breakdown = (items, multiplier = null) => ({ items, multiplier });

test('orders known items and preserves other item order while omitting zero items', () => {
  const rows = buildCostBreakdown(4, breakdown([
    item('audio_tokens', 3, 1), item('prompt_write_cached_tokens', 1, 1, 'one_hour'),
    item('completion_tokens', 2, 1), item('zero', 0, 0), item('prompt_tokens', 4, 1),
    item('prompt_cached_tokens', 5, 0), item('image_tokens', 0, 1),
  ]));
  assert.deepEqual(rows.items.map(({ itemCode }) => itemCode), [
    'prompt_tokens', 'completion_tokens', 'prompt_cached_tokens', 'prompt_write_cached_tokens', 'audio_tokens', 'image_tokens',
  ]);
});

test('unit price uses the stored subtotal per million tokens and is absent when quantity is zero', () => {
  const rows = buildCostBreakdown(0.75, breakdown([item('prompt_tokens', 250_000, 0.5), item('completion_tokens', 0, 0.25)]));
  assert.equal(rows.items[0].unitPrice, 2);
  assert.equal(rows.items[1].unitPrice, null);
});

test('unknown and identity multipliers do not show a base; non-unit and zero multipliers do', () => {
  for (const multiplier of [null, 1]) {
    const rows = buildCostBreakdown(2, breakdown([], multiplier));
    assert.equal(rows.multiplier, null);
    assert.equal(rows.baseTotal, null);
    assert.equal(rows.total, 2);
  }
  assert.deepEqual(
    (({ multiplier, baseTotal, total }) => ({ multiplier, baseTotal, total }))(buildCostBreakdown(2, breakdown([], 2))),
    { multiplier: 2, baseTotal: 1, total: 2 }
  );
  const free = buildCostBreakdown(0, breakdown([], 0));
  assert.equal(free.multiplier, 0);
  assert.equal(free.baseTotal, null);
  assert.equal(free.total, 0);
});

test('only multi-segment tiers receive display rows and per-tier unit prices', () => {
  const single = item('prompt_tokens', 10, 1, null, [{ upTo: null, units: 10, subtotal: 1 }]);
  const tiered = item('completion_tokens', 30, 3, null, [
    { upTo: 10, units: 10, subtotal: 0.5 }, { upTo: null, units: 20, subtotal: 2.5 },
  ]);
  const rows = buildCostBreakdown(4, breakdown([single, tiered]));
  assert.deepEqual(rows.items[0].tiers, []);
  assert.deepEqual(rows.items[1].tiers, [
    { upTo: 10, units: 10, subtotal: 0.5, unitPrice: 50_000 },
    { upTo: null, units: 20, subtotal: 2.5, unitPrice: 125_000 },
  ]);
});
