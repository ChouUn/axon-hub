import assert from 'node:assert/strict';
import test from 'node:test';
import { displayCost } from './cost/format.ts';

const currency = (value) => `$${value.toFixed(6)}`;
const unpriced = (count) => `Includes ${count} unpriced records`;

test('entirely unpriced usage displays only a dash', () => {
  assert.deepEqual(displayCost(null, 3, currency, unpriced), { amount: '—', unpriced: null });
});

test('partly unpriced usage retains priced amount and count', () => {
  assert.deepEqual(displayCost(1.25, 2, currency, unpriced), { amount: '$1.250000', unpriced: 'Includes 2 unpriced records' });
});

test('free and empty usage show real zero without marker', () => {
  assert.deepEqual(displayCost(0, 0, currency, unpriced), { amount: '$0.000000', unpriced: null });
});
