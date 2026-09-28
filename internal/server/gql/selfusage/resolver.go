package selfusage

import (
	"fmt"
	"time"

	"github.com/99designs/gqlgen/graphql"

	"github.com/looplj/axonhub/internal/server/biz"
)

// Resolver holds the service used by the standalone self-usage schema.
type Resolver struct {
	selfUsage *biz.SelfUsageService
}

func NewSchema(service *biz.SelfUsageService) graphql.ExecutableSchema {
	return NewExecutableSchema(Config{Resolvers: &Resolver{selfUsage: service}})
}

// Validate dates before calling the service; the service also validates its
// own public entry points and applies the configured timezone to the interval.
func validateRange(start, end string) error {
	first, err := time.Parse("2006-01-02", start)
	if err != nil || first.Format("2006-01-02") != start {
		return fmt.Errorf("invalid_range: start must be YYYY-MM-DD")
	}
	last, err := time.Parse("2006-01-02", end)
	if err != nil || last.Format("2006-01-02") != end {
		return fmt.Errorf("invalid_range: end must be YYYY-MM-DD")
	}
	if last.Before(first) || last.After(first.AddDate(0, 0, 89)) {
		return fmt.Errorf("invalid_range: range must contain 1 to 90 days")
	}
	return nil
}

func toSummary(value biz.SelfUsageSummary) *SelfUsageSummary {
	return &SelfUsageSummary{
		SuccessRequests: value.SuccessRequests, FailedRequests: value.FailedRequests,
		InputTokens: value.InputTokens, OutputTokens: value.OutputTokens,
		CacheReadTokens: value.CacheReadTokens, CacheWriteTokens: value.CacheWriteTokens,
		ReasoningTokens: value.ReasoningTokens, TotalTokens: value.TotalTokens,
		Cost: value.Cost, UsageRecords: value.UsageRecords, UnpricedRecords: value.UnpricedRecords,
	}
}

func toRequest(value biz.SelfUsageRequest) *SelfUsageRequest {
	return &SelfUsageRequest{
		ID: value.ID, CreatedAt: value.CreatedAt, Model: value.Model,
		Status: SelfUsageRequestStatus(value.Status), Stream: value.Stream,
		LatencyMs: value.LatencyMs, FirstTokenLatencyMs: value.FirstTokenLatencyMs,
		InputTokens: value.InputTokens, OutputTokens: value.OutputTokens,
		CacheReadTokens: value.CacheReadTokens, CacheWriteTokens: value.CacheWriteTokens,
		ReasoningTokens: value.ReasoningTokens, TotalTokens: value.TotalTokens,
		Cost: value.Cost, UsageRecords: value.UsageRecords, UnpricedRecords: value.UnpricedRecords,
	}
}
