import type { ModelPrice } from '@/features/channels/data/schema';
import type { ProviderModel } from './providers.schema';

/**
 * Reasoning effort levels AxonHub passes through unchanged (see llm/reasoning.go).
 * A model card lists the subset its upstream accepts.
 */
export const REASONING_EFFORTS = ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'] as const;

export type ReasoningEffort = (typeof REASONING_EFFORTS)[number];

const MODEL_SUFFIX_EFFORTS: Record<string, true> = { low: true, medium: true, high: true, xhigh: true, max: true };

export function splitReasoningEffortModelSuffix(model: string): { base: string; effort: string } | null {
  const separator = model.lastIndexOf('-');
  if (separator <= 0 || separator === model.length - 1) return null;

  const base = model.slice(0, separator);
  const effort = model.slice(separator + 1).toLowerCase();
  // Own keys only: inherited names such as "constructor" are not efforts.
  if (!Object.hasOwn(MODEL_SUFFIX_EFFORTS, effort)) return null;

  const lastSegment = model.slice(model.lastIndexOf('/') + 1).trim().toLowerCase();
  if (effort === 'max' && lastSegment.startsWith('qwen')) return null;

  return { base, effort };
}

/** Mirrors the backend rule: a price needs items, volume tiers or schedule overrides. */
export function hasStandardPrice(price: ModelPrice | null | undefined): price is ModelPrice {
  return !!price && (price.items.length > 0 || !!price.volumeTiers?.length || !!price.schedule?.overrides.length);
}

/** Returns the first usable standard price, trying candidates in order. */
export function firstStandardPrice(
  candidates: readonly string[],
  pricesById: ReadonlyMap<string, ModelPrice | null | undefined>
): ModelPrice | undefined {
  for (const candidate of candidates) {
    const price = pricesById.get(candidate);
    if (hasStandardPrice(price)) return price;
  }
  return undefined;
}

export function priceModelIdCandidates(modelId: string, autoTrimedPrefixes?: readonly string[] | null): string[] {
  const candidates: string[] = [];
  const seen = new Set<string>();
  const addCandidate = (candidate: string) => {
    if (!seen.has(candidate)) {
      seen.add(candidate);
      candidates.push(candidate);
    }
  };

  addCandidate(modelId);
  const exactSplit = splitReasoningEffortModelSuffix(modelId);
  if (exactSplit) addCandidate(exactSplit.base);

  for (const rawPrefix of autoTrimedPrefixes ?? []) {
    const prefix = rawPrefix.trim();
    if (!prefix || !modelId.startsWith(`${prefix}/`)) continue;

    const trimmedModelId = modelId.slice(prefix.length + 1);
    if (!trimmedModelId) continue;
    addCandidate(trimmedModelId);
    const trimmedSplit = splitReasoningEffortModelSuffix(trimmedModelId);
    if (trimmedSplit) addCandidate(trimmedSplit.base);
  }

  return candidates;
}

/**
 * Derives the supported levels from a catalog model's `reasoning_options`.
 *
 * Only the `effort` option enumerates levels. A `toggle` or `budget_tokens`
 * option says nothing about which levels the upstream accepts, and most models
 * carry no options at all, so those stay unknown rather than guessed.
 */
export function deriveReasoningEfforts(options: ProviderModel['reasoning_options']): ReasoningEffort[] | undefined {
  const values = options?.find((option) => option.type === 'effort')?.values?.filter((value): value is string => !!value);
  if (!values?.length) return undefined;

  const efforts = REASONING_EFFORTS.filter((effort) => values.includes(effort));

  return efforts.length ? efforts : undefined;
}
