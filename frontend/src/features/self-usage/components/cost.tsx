export type MoneyFormatter = (cost: number | null, unpricedRecords: number) => { amount: string; unpriced: string | null };

export function Cost({ value, unpriced, money }: { value: number | null; unpriced: number; money: MoneyFormatter }) {
  const shown = money(value, unpriced);
  return (
    <span className='inline-flex flex-col gap-1'>
      <span>{shown.amount}</span>
      {shown.unpriced && <span className='text-muted-foreground text-xs'>{shown.unpriced}</span>}
    </span>
  );
}
