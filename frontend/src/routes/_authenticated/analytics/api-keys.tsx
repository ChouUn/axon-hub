import { createFileRoute } from '@tanstack/react-router';
import { RouteGuard } from '@/components/route-guard';
import APIKeyAnalyticsPage from '@/features/analytics/api-key-analytics';

function ProtectedAPIKeyAnalytics() {
  return (
    <RouteGuard requiredScopes={['read_dashboard']} scopeLevel='system'>
      <APIKeyAnalyticsPage />
    </RouteGuard>
  );
}

export const Route = createFileRoute('/_authenticated/analytics/api-keys')({
  component: ProtectedAPIKeyAnalytics,
});
