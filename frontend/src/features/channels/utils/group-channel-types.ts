export interface ChannelTypeCountLike {
  type: string;
  count: number;
}

export interface ChannelTypeGroup {
  /** Stable key used as the tab identity. */
  key: string;
  /** Provider the group belongs to; used for label and icon lookup. */
  provider: string;
  /** Channel types that fall into this group, in input order. */
  types: string[];
  totalCount: number;
}

/**
 * Groups channel type counts by provider so that protocol variants of the same
 * vendor (`deepseek` / `deepseek_anthropic`, `moonshot` / `moonshot_coding`) land
 * in a single tab. Types missing from the mapping keep their own group so they
 * stay reachable instead of silently disappearing.
 */
export function groupChannelTypesByProvider(
  typeCounts: ChannelTypeCountLike[],
  typeToProvider: Record<string, string>
): ChannelTypeGroup[] {
  const groups = new Map<string, ChannelTypeGroup>();

  for (const { type, count } of typeCounts) {
    const provider = typeToProvider[type] ?? type;
    const existing = groups.get(provider);
    if (existing) {
      existing.types.push(type);
      existing.totalCount += count;
      continue;
    }
    groups.set(provider, { key: provider, provider, types: [type], totalCount: count });
  }

  return Array.from(groups.values()).sort((a, b) => b.totalCount - a.totalCount || a.key.localeCompare(b.key));
}
