export type CostDisplay = { amount: string; unpriced: string | null };

export function displayCost(
  cost: number | null,
  unpricedRecords: number,
  formatCurrency: (value: number) => string,
  formatUnpriced: (count: number) => string
): CostDisplay {
  return {
    amount: cost === null ? '—' : formatCurrency(cost),
    // Entirely unpriced usage is already "—"; the count only qualifies a partial known cost.
    unpriced: cost !== null && unpricedRecords > 0 ? formatUnpriced(unpricedRecords) : null,
  };
}
