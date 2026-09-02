export const IMAGE_PRIMARY_API_FORMAT = 'openai/image_generation';

export interface ChannelTypeCountLike {
  type: string;
  count: number;
  primaryApiFormat?: string | null;
}

export interface ChannelTypeGroup {
  /** Stable key used as the tab identity. */
  key: string;
  /** Provider the group belongs to; used for label and icon lookup. */
  provider: string;
  /** Channel types that fall into this group, in input order. */
  types: string[];
  totalCount: number;
  /** Set when this group is the image-generation split of a vendor. */
  primaryApiFormat?: string;
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

  for (const { type, count, primaryApiFormat } of typeCounts) {
    const provider = typeToProvider[type] ?? type;
    const isImage = primaryApiFormat === IMAGE_PRIMARY_API_FORMAT;
    const key = isImage ? `${provider}:image` : provider;
    const existing = groups.get(key);
    if (existing) {
      existing.types.push(type);
      existing.totalCount += count;
      continue;
    }
    const group: ChannelTypeGroup = { key, provider, types: [type], totalCount: count };
    if (isImage) {
      group.primaryApiFormat = IMAGE_PRIMARY_API_FORMAT;
    }
    groups.set(key, group);
  }

  return Array.from(groups.values()).sort((a, b) => b.totalCount - a.totalCount || a.key.localeCompare(b.key));
}
