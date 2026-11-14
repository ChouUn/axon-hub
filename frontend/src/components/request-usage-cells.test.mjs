import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { IconCoin } from '@tabler/icons-react';
import ts from 'typescript';
import { buildCostBreakdown, isModelLibraryPriceReference } from './request-usage-cost-breakdown.ts';

const source = readFileSync(new URL('./request-usage-cells.tsx', import.meta.url), 'utf8');
const ast = ts.createSourceFile('request-usage-cells.tsx', source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
const cell = ast.statements.find((node) => ts.isFunctionDeclaration(node) && node.name?.text === 'CostCell');
assert.ok(cell);
const { outputText } = ts.transpileModule(cell.getText(ast).replace(/^export\s+/, ''), {
  compilerOptions: { jsx: ts.JsxEmit.React, module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2023 },
});
const translations = {
  ...JSON.parse(readFileSync(new URL('../locales/en/requests.json', import.meta.url), 'utf8')),
  ...JSON.parse(readFileSync(new URL('../locales/en/usageLogs.json', import.meta.url), 'utf8')),
};
const t = (key, options) => key === 'currencies.format'
  ? `${options.currency} ${options.val.toFixed(options.minimumFractionDigits)}`
  : translations[key] ?? key;
// As in quota-badges.test.mjs, expose tooltip children in SSR without Radix's
// closed-state portal so assertions cover the real cell's marker and amounts.
const passChildren = ({ children }) => children;
const CostCell = new Function(
  'React', 'useTranslation', 'Badge', 'Tooltip', 'TooltipTrigger', 'TooltipContent', 'IconCoin',
  'buildCostBreakdown', 'isModelLibraryPriceReference',
  `${outputText}\nreturn CostCell;`
)(
  React, () => ({ t, i18n: { language: 'en' } }), passChildren, passChildren, passChildren, passChildren, IconCoin,
  buildCostBreakdown, isModelLibraryPriceReference
);
const renderCost = (props) => renderToStaticMarkup(React.createElement(CostCell, {
  total: 4, currencyCode: 'USD', breakdown: null, ...props,
}));
const sourceLabel = translations['usageLogs.modelLibraryPriceHint'];

for (const costPriceReferenceID of ['model_library:model', 'model_library@0.5:provider:model']) {
  test(`cost cell displays the model library marker for ${costPriceReferenceID}`, () => {
    for (const breakdown of [null, { items: [], multiplier: 0.5 }]) {
      const html = renderCost({ costPriceReferenceID, breakdown });
      assert.ok(html.includes(`aria-label="${sourceLabel}"`));
      assert.match(html, /text-amber-600/);
      assert.match(html, /<svg/);
      assert.ok(html.includes(sourceLabel));
    }
  });
}

test('cost cells without a library reference keep their cost display without a source marker', () => {
  for (const costPriceReferenceID of [undefined, null, 'channel-price-version', 'model_library@invalid:model']) {
    const html = renderCost({ costPriceReferenceID, breakdown: { items: [], multiplier: 2 } });
    assert.ok(html.includes('USD 4.000000'));
    assert.equal(html.includes(sourceLabel), false);
    assert.doesNotMatch(html, /<svg/);
  }
});

test('unpriced cost cells omit the model library marker and breakdown', () => {
  const html = renderCost({ total: null, costPriceReferenceID: 'model_library:model', emptyLabel: '-' });
  assert.equal(html, '<span class="font-mono text-xs">-</span>');
});

test('model library costs display the stored base total, multiplier and billed total', () => {
  for (const [multiplier, total, baseTotal] of [[0.5, 4, 8], [2, 4, 2], [0, 0, null]]) {
    const html = renderCost({
      total,
      costPriceReferenceID: `model_library@${multiplier}:model`,
      breakdown: { items: [], multiplier },
    });
    assert.ok(html.includes(`aria-label="${sourceLabel}"`));
    assert.ok(html.includes(`<span>${translations['requests.costBreakdown.baseTotal']}</span><span class="font-mono">${baseTotal === null ? '-' : `USD ${baseTotal.toFixed(6)}`}</span>`));
    assert.ok(html.includes(`<span>×${multiplier}</span>`));
    assert.ok(html.includes(`<span>${translations['requests.costBreakdown.total']}</span><span class="font-mono">USD ${total.toFixed(6)}</span>`));
  }
});
