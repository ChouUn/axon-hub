import assert from 'node:assert/strict';
import test from 'node:test';

import { groupChannelTypesByProvider, IMAGE_PRIMARY_API_FORMAT } from './utils/group-channel-types.ts';

const typeToProvider = {
  deepseek: 'deepseek',
  deepseek_anthropic: 'deepseek',
  moonshot: 'moonshot',
  moonshot_anthropic: 'moonshot',
  moonshot_coding: 'moonshot',
  openai: 'openai',
  openai_responses: 'openai',
};

test('protocol variants of one vendor collapse into a single provider group', () => {
  const groups = groupChannelTypesByProvider(
    [
      { type: 'deepseek_anthropic', count: 3 },
      { type: 'deepseek', count: 2 },
      { type: 'moonshot_coding', count: 1 },
      { type: 'moonshot', count: 1 },
      { type: 'moonshot_anthropic', count: 1 },
    ],
    typeToProvider
  );

  assert.deepEqual(groups, [
    { key: 'deepseek', provider: 'deepseek', types: ['deepseek_anthropic', 'deepseek'], totalCount: 5 },
    { key: 'moonshot', provider: 'moonshot', types: ['moonshot_coding', 'moonshot', 'moonshot_anthropic'], totalCount: 3 },
  ]);
});

test('a lone protocol variant still groups under its provider', () => {
  const groups = groupChannelTypesByProvider([{ type: 'deepseek_anthropic', count: 4 }], typeToProvider);

  assert.deepEqual(groups, [{ key: 'deepseek', provider: 'deepseek', types: ['deepseek_anthropic'], totalCount: 4 }]);
});

test('unmapped types keep their own group instead of disappearing', () => {
  const groups = groupChannelTypesByProvider(
    [
      { type: 'openai', count: 1 },
      { type: 'brand_new_type', count: 1 },
    ],
    typeToProvider
  );

  assert.deepEqual(groups, [
    { key: 'brand_new_type', provider: 'brand_new_type', types: ['brand_new_type'], totalCount: 1 },
    { key: 'openai', provider: 'openai', types: ['openai'], totalCount: 1 },
  ]);
});

test('image-generation channels split into a sibling vendor group', () => {
  const groups = groupChannelTypesByProvider(
    [
      { type: 'openai', count: 4 },
      { type: 'openai', count: 2, primaryApiFormat: IMAGE_PRIMARY_API_FORMAT },
      { type: 'openai_responses', count: 1 },
    ],
    typeToProvider
  );

  assert.deepEqual(groups, [
    { key: 'openai', provider: 'openai', types: ['openai', 'openai_responses'], totalCount: 5 },
    {
      key: 'openai:image',
      provider: 'openai',
      types: ['openai'],
      totalCount: 2,
      primaryApiFormat: IMAGE_PRIMARY_API_FORMAT,
    },
  ]);
});

test('groups sort by total count descending, then by key', () => {
  const groups = groupChannelTypesByProvider(
    [
      { type: 'openai', count: 1 },
      { type: 'moonshot', count: 5 },
      { type: 'deepseek', count: 1 },
    ],
    typeToProvider
  );

  assert.deepEqual(
    groups.map(({ key }) => key),
    ['moonshot', 'deepseek', 'openai']
  );
});
