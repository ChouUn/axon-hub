import { createFileRoute } from '@tanstack/react-router';
import { RouteGuard } from '@/components/route-guard';
import ModelAnalyticsPage from '@/features/analytics/model-analytics';

function ProtectedModelAnalytics() {
  return (
    <RouteGuard requiredScopes={['read_dashboard']} scopeLevel='system'>
      <ModelAnalyticsPage />
    </RouteGuard>
  );
}

export const Route = createFileRoute('/_authenticated/analytics/models')({
  component: ProtectedModelAnalytics,
});
