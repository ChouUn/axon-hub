import { createFileRoute } from '@tanstack/react-router';
import { SelfUsagePage } from '@/features/self-usage';

export const Route = createFileRoute('/self-usage')({ component: SelfUsagePage });
