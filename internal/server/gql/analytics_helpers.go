package gql

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/samber/lo"

	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/apikey"
	"github.com/looplj/axonhub/internal/ent/apikeyprofiletemplate"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/usagelog"
	"github.com/looplj/axonhub/internal/objects"
)

// parseDateStr 解析 YYYY-MM-DD 日期字符串为系统时区午夜时间（保持 loc 时区）
// 与仪表盘 GetCalendarPeriods 中 todayStart 的构建方式一致：
// time.Date(year, month, day, 0, 0, 0, 0, loc)
// 不转 UTC，因为后续 fill-missing-dates 循环需要 loc 时区的 Year/Month/Day.
func parseDateStr(dateStr string, loc *time.Location) time.Time {
	parts := strings.Split(dateStr, "-")
	if len(parts) != 3 {
		return time.Time{}
	}
	y, m, d := 0, 0, 0
	for i, p := range parts {
		n := 0
		for _, c := range p {
			n = n*10 + int(c-'0')
		}
		switch i {
		case 0:
			y = n
		case 1:
			m = n
		case 2:
			d = n
		}
	}
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, loc)
}

func (r *queryResolver) buildAnalyticsWhere(s *sql.Selector, filter *AnalyticsFilter, apiKeyIDs []int, hasAPIKeyFilter bool, loc *time.Location) {
	if filter == nil {
		return
	}

	if filter.StartTime != nil {
		startDate := parseDateStr(*filter.StartTime, loc)
		if !startDate.IsZero() {
			// 同仪表盘：本地午夜转 UTC 再比较，数据库 created_at 是 UTC
			s.Where(sql.GTE(s.C(usagelog.FieldCreatedAt), startDate.UTC()))
		}
	}

	if filter.EndTime != nil {
		endDate := parseDateStr(*filter.EndTime, loc)
		if !endDate.IsZero() {
			endDateNext := endDate.AddDate(0, 0, 1)
			s.Where(sql.LT(s.C(usagelog.FieldCreatedAt), endDateNext.UTC()))
		}
	}

	if len(filter.ProjectIDs) > 0 {
		ids := lo.Map(filter.ProjectIDs, func(g *objects.GUID, _ int) int { return g.ID })
		s.Where(sql.InInts(usagelog.FieldProjectID, ids...))
	}

	if len(filter.ChannelIDs) > 0 {
		ids := lo.Map(filter.ChannelIDs, func(g *objects.GUID, _ int) int { return g.ID })
		s.Where(sql.InInts(usagelog.FieldChannelID, ids...))
	}

	if len(filter.ModelIDs) > 0 {
		vals := make([]any, len(filter.ModelIDs))
		for i, v := range filter.ModelIDs {
			vals[i] = v
		}
		s.Where(sql.In(usagelog.FieldModelID, vals...))
	}

	// API key / user / template filtering:
	// - apiKeyIDs > 0: filter by specific API keys
	// - apiKeyIDs == 0 && hasAPIKeyFilter: a key-related filter matched no keys → return empty
	// - apiKeyIDs == 0 && !hasAPIKeyFilter: no key-related filter → show all
	if len(apiKeyIDs) > 0 {
		s.Where(sql.InInts(usagelog.FieldAPIKeyID, apiKeyIDs...))
	} else if hasAPIKeyFilter {
		s.Where(sql.False())
	}
}

// resolveFilterAPIKeyIDs resolves the effective API key IDs from explicit key,
// user, and profile-template filters. Multiple key-related filters use AND logic.
func (r *queryResolver) resolveFilterAPIKeyIDs(ctx context.Context, filter *AnalyticsFilter) ([]int, bool) {
	if filter == nil {
		return nil, false
	}

	hasExplicitKeys := len(filter.APIKeyIDs) > 0
	hasUserFilter := len(filter.UserIDs) > 0
	hasTemplateFilter := len(filter.TemplateIDs) > 0

	if !hasExplicitKeys && !hasUserFilter && !hasTemplateFilter {
		return nil, false
	}

	filterSets := make([][]int, 0, 3)
	if hasExplicitKeys {
		filterSets = append(filterSets, lo.Map(filter.APIKeyIDs, func(g *objects.GUID, _ int) int { return g.ID }))
	}

	if hasUserFilter {
		userIDs := lo.Map(filter.UserIDs, func(g *objects.GUID, _ int) int { return g.ID })
		apiKeys, err := r.client.APIKey.Query().
			Where(apikey.UserIDIn(userIDs...)).
			All(ctx)
		if err != nil {
			return nil, hasUserFilter
		}
		filterSets = append(filterSets, lo.Map(apiKeys, func(ak *ent.APIKey, _ int) int { return ak.ID }))
	}

	if hasTemplateFilter {
		templateIDs := make(map[int]struct{}, len(filter.TemplateIDs))
		for _, templateID := range filter.TemplateIDs {
			templateIDs[templateID.ID] = struct{}{}
		}

		apiKeyQuery := r.client.APIKey.Query()
		if projectID, ok := contexts.GetProjectID(ctx); ok {
			apiKeyQuery = apiKeyQuery.Where(apikey.ProjectIDEQ(projectID))
		}

		apiKeys, err := apiKeyQuery.All(ctx)
		if err != nil {
			return nil, true
		}

		templateKeyIDs := make([]int, 0)
		for _, apiKey := range apiKeys {
			if apiKeyUsesTemplate(apiKey, templateIDs) {
				templateKeyIDs = append(templateKeyIDs, apiKey.ID)
			}
		}
		filterSets = append(filterSets, templateKeyIDs)
	}

	return intersectAPIKeyIDs(filterSets...), true
}

func apiKeyUsesTemplate(apiKey *ent.APIKey, templateIDs map[int]struct{}) bool {
	if apiKey == nil || apiKey.Profiles == nil {
		return false
	}

	for _, profile := range apiKey.Profiles.Profiles {
		if profile.TemplateID == nil {
			continue
		}

		if _, ok := templateIDs[*profile.TemplateID]; ok {
			return true
		}
	}

	return false
}

func intersectAPIKeyIDs(sets ...[]int) []int {
	if len(sets) == 0 {
		return nil
	}

	intersection := make(map[int]struct{}, len(sets[0]))
	for _, id := range sets[0] {
		intersection[id] = struct{}{}
	}

	for _, ids := range sets[1:] {
		current := make(map[int]struct{}, len(ids))
		for _, id := range ids {
			current[id] = struct{}{}
		}

		for id := range intersection {
			if _, ok := current[id]; !ok {
				delete(intersection, id)
			}
		}
	}

	return lo.Keys(intersection)
}

func trimSpace(s string) string {
	return strings.TrimSpace(s)
}

// dimStats holds aggregated dimension statistics from raw SQL queries.
type dimStats struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	RequestCount int     `json:"request_count"`
	InputTokens  int64   `json:"input_tokens"`
	CachedTokens int64   `json:"cached_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	Cost         float64 `json:"cost"`
}

type apiKeyAnalyticsRaw struct {
	APIKeyID     int     `json:"api_key_id"`
	ModelID      string  `json:"model_id"`
	RequestCount int     `json:"request_count"`
	TotalTokens  int64   `json:"total_tokens"`
	Cost         float64 `json:"cost"`
}

func (r *queryResolver) queryAnalyticsAPIKeyTemplates(ctx context.Context) ([]*AnalyticsAPIKeyTemplate, error) {
	query := r.client.APIKeyProfileTemplate.Query()
	if projectID, ok := contexts.GetProjectID(ctx); ok {
		query = query.Where(apikeyprofiletemplate.ProjectIDEQ(projectID))
	}

	templates, err := query.Order(ent.Asc(apikeyprofiletemplate.FieldName)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get API key profile templates: %w", err)
	}

	return lo.Map(templates, func(template *ent.APIKeyProfileTemplate, _ int) *AnalyticsAPIKeyTemplate {
		return &AnalyticsAPIKeyTemplate{
			ID:   objects.GUID{Type: ent.TypeAPIKeyProfileTemplate, ID: template.ID},
			Name: template.Name,
		}
	}), nil
}

func (r *queryResolver) queryAnalyticsAPIKeyStats(ctx context.Context, filter *AnalyticsFilter) ([]*AnalyticsAPIKeyStat, error) {
	loc := r.systemService.TimeLocation(ctx)
	apiKeyIDs, hasAPIKeyFilter := r.resolveFilterAPIKeyIDs(ctx, filter)
	var rawResults []apiKeyAnalyticsRaw

	err := r.client.UsageLog.Query().
		Where(usagelog.APIKeyIDNotNil()).
		Modify(func(s *sql.Selector) {
			r.buildAnalyticsWhere(s, filter, apiKeyIDs, hasAPIKeyFilter, loc)
			if projectID, ok := contexts.GetProjectID(ctx); ok {
				s.Where(sql.EQ(s.C(usagelog.FieldProjectID), projectID))
			}

			s.Select(
				s.C(usagelog.FieldAPIKeyID),
				s.C(usagelog.FieldModelID),
				sql.As(sql.Count(s.C(usagelog.FieldID)), "request_count"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldTotalTokens)), "total_tokens"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldTotalCost)), "cost"),
			).
				GroupBy(s.C(usagelog.FieldAPIKeyID), s.C(usagelog.FieldModelID))
		}).
		Scan(ctx, &rawResults)
	if err != nil {
		return nil, fmt.Errorf("failed to get API key analytics stats: %w", err)
	}

	if len(rawResults) == 0 {
		return []*AnalyticsAPIKeyStat{}, nil
	}

	apiKeyIDsFromResults := lo.Map(rawResults, func(item apiKeyAnalyticsRaw, _ int) int { return item.APIKeyID })
	apiKeyQuery := r.client.APIKey.Query().Where(apikey.IDIn(apiKeyIDsFromResults...))
	if projectID, ok := contexts.GetProjectID(ctx); ok {
		apiKeyQuery = apiKeyQuery.Where(apikey.ProjectIDEQ(projectID))
	}
	apiKeys, err := apiKeyQuery.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get API keys for analytics stats: %w", err)
	}

	apiKeyNames := lo.SliceToMap(apiKeys, func(apiKey *ent.APIKey) (int, string) {
		return apiKey.ID, apiKey.Name
	})
	groups := make(map[int]*AnalyticsAPIKeyStat, len(apiKeys))
	for _, raw := range rawResults {
		name := fmt.Sprintf("API Key #%d", raw.APIKeyID)
		if resolvedName, ok := apiKeyNames[raw.APIKeyID]; ok {
			name = resolvedName
		}

		group, exists := groups[raw.APIKeyID]
		if !exists {
			group = &AnalyticsAPIKeyStat{
				ID:   objects.GUID{Type: ent.TypeAPIKey, ID: raw.APIKeyID},
				Name: name,
			}
			groups[raw.APIKeyID] = group
		}

		group.RequestCount += raw.RequestCount
		group.TotalTokens = safeIntFromInt64(int64(group.TotalTokens) + raw.TotalTokens)
		group.Cost += raw.Cost
		group.Models = append(group.Models, &AnalyticsAPIKeyModelStat{
			ID:           raw.ModelID,
			Name:         raw.ModelID,
			RequestCount: raw.RequestCount,
			TotalTokens:  safeIntFromInt64(raw.TotalTokens),
			Cost:         raw.Cost,
		})
	}

	results := lo.Values(groups)
	for _, group := range results {
		sort.SliceStable(group.Models, func(i, j int) bool {
			left, right := group.Models[i], group.Models[j]
			if left.RequestCount != right.RequestCount {
				return left.RequestCount > right.RequestCount
			}
			if left.TotalTokens != right.TotalTokens {
				return left.TotalTokens > right.TotalTokens
			}
			return left.Name < right.Name
		})
	}
	sortAPIKeyStatsByCost(results)

	return results, nil
}

// sortAPIKeyStatsByCost orders API keys by spend for the ranking table.
func sortAPIKeyStatsByCost(results []*AnalyticsAPIKeyStat) {
	sort.SliceStable(results, func(i, j int) bool {
		left, right := results[i], results[j]
		if left.Cost != right.Cost {
			return left.Cost > right.Cost
		}
		if left.RequestCount != right.RequestCount {
			return left.RequestCount > right.RequestCount
		}
		if left.TotalTokens != right.TotalTokens {
			return left.TotalTokens > right.TotalTokens
		}
		return left.Name < right.Name
	})
}

func (r *queryResolver) queryChannelStats(ctx context.Context, filter *AnalyticsFilter, apiKeyIDs []int, hasUserFilter bool, loc *time.Location) ([]dimStats, error) {
	type channelStatsRaw struct {
		ChannelID    int     `json:"channel_id"`
		Name         string  `json:"name"`
		RequestCount int     `json:"request_count"`
		InputTokens  int64   `json:"input_tokens"`
		CachedTokens int64   `json:"cached_tokens"`
		OutputTokens int64   `json:"output_tokens"`
		TotalTokens  int64   `json:"total_tokens"`
		Cost         float64 `json:"cost"`
	}

	var rawResults []channelStatsRaw

	err := r.client.UsageLog.Query().
		Modify(func(s *sql.Selector) {
			channelTable := sql.Table(channel.Table)
			s.Join(channelTable).On(
				s.C(usagelog.FieldChannelID),
				channelTable.C(channel.FieldID),
			)
			s.Where(sql.EQ(channelTable.C(channel.FieldDeletedAt), 0))

			r.buildAnalyticsWhere(s, filter, apiKeyIDs, hasUserFilter, loc)

			s.Select(
				sql.As(s.C(usagelog.FieldChannelID), "channel_id"),
				sql.As(channelTable.C(channel.FieldName), "name"),
				sql.As(sql.Count(s.C(usagelog.FieldID)), "request_count"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldPromptTokens)), "input_tokens"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldPromptCachedTokens)), "cached_tokens"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldCompletionTokens)), "output_tokens"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldTotalTokens)), "total_tokens"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldTotalCost)), "cost"),
			).
				GroupBy(s.C(usagelog.FieldChannelID), channelTable.C(channel.FieldName)).
				OrderBy(sql.Desc("total_tokens"))
		}).
		Scan(ctx, &rawResults)
	if err != nil {
		return nil, fmt.Errorf("failed to get analytics stats by channel: %w", err)
	}

	results := make([]dimStats, 0, len(rawResults))

	for _, raw := range rawResults {
		results = append(results, dimStats{
			ID:           fmt.Sprintf("%d", raw.ChannelID),
			Name:         raw.Name,
			RequestCount: raw.RequestCount,
			InputTokens:  raw.InputTokens,
			CachedTokens: raw.CachedTokens,
			OutputTokens: raw.OutputTokens,
			TotalTokens:  raw.TotalTokens,
			Cost:         raw.Cost,
		})
	}

	return results, nil
}

func (r *queryResolver) queryModelStats(ctx context.Context, filter *AnalyticsFilter, apiKeyIDs []int, hasUserFilter bool, loc *time.Location) ([]dimStats, error) {
	var results []dimStats

	err := r.client.UsageLog.Query().
		Modify(func(s *sql.Selector) {
			r.buildAnalyticsWhere(s, filter, apiKeyIDs, hasUserFilter, loc)

			s.Select(
				sql.As(s.C(usagelog.FieldModelID), "id"),
				sql.As(s.C(usagelog.FieldModelID), "name"),
				sql.As(sql.Count(s.C(usagelog.FieldID)), "request_count"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldPromptTokens)), "input_tokens"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldPromptCachedTokens)), "cached_tokens"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldCompletionTokens)), "output_tokens"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldTotalTokens)), "total_tokens"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldTotalCost)), "cost"),
			).
				GroupBy(s.C(usagelog.FieldModelID)).
				OrderBy(sql.Desc("total_tokens"))
		}).
		Scan(ctx, &results)
	if err != nil {
		return nil, fmt.Errorf("failed to get analytics stats by model: %w", err)
	}

	return results, nil
}

func (r *queryResolver) queryAPIKeyStats(ctx context.Context, filter *AnalyticsFilter, apiKeyIDs []int, hasUserFilter bool, loc *time.Location) ([]dimStats, error) {
	type apiKeyStatsRaw struct {
		APIKeyID     int     `json:"api_key_id"`
		RequestCount int     `json:"request_count"`
		InputTokens  int64   `json:"input_tokens"`
		CachedTokens int64   `json:"cached_tokens"`
		OutputTokens int64   `json:"output_tokens"`
		TotalTokens  int64   `json:"total_tokens"`
		Cost         float64 `json:"cost"`
	}

	var rawResults []apiKeyStatsRaw

	err := r.client.UsageLog.Query().
		Where(usagelog.APIKeyIDNotNil()).
		Modify(func(s *sql.Selector) {
			r.buildAnalyticsWhere(s, filter, apiKeyIDs, hasUserFilter, loc)

			s.Select(
				sql.As(s.C(usagelog.FieldAPIKeyID), "api_key_id"),
				sql.As(sql.Count(s.C(usagelog.FieldID)), "request_count"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldPromptTokens)), "input_tokens"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldPromptCachedTokens)), "cached_tokens"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldCompletionTokens)), "output_tokens"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldTotalTokens)), "total_tokens"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldTotalCost)), "cost"),
			).
				GroupBy(s.C(usagelog.FieldAPIKeyID)).
				OrderBy(sql.Desc("total_tokens"))
		}).
		Scan(ctx, &rawResults)
	if err != nil {
		return nil, fmt.Errorf("failed to get analytics stats by apiKey: %w", err)
	}

	var results []dimStats

	if len(rawResults) > 0 {
		akIDs := lo.Map(rawResults, func(item apiKeyStatsRaw, _ int) int { return item.APIKeyID })
		apiKeys, qErr := r.client.APIKey.Query().Where(apikey.IDIn(akIDs...)).All(ctx)
		if qErr != nil {
			return nil, fmt.Errorf("failed to get API key details: %w", qErr)
		}
		apiKeyMap := lo.SliceToMap(apiKeys, func(ak *ent.APIKey) (int, *ent.APIKey) { return ak.ID, ak })

		for _, raw := range rawResults {
			name := fmt.Sprintf("API Key #%d", raw.APIKeyID)
			if ak, ok := apiKeyMap[raw.APIKeyID]; ok {
				name = ak.Name
			}
			results = append(results, dimStats{
				ID:           fmt.Sprintf("%d", raw.APIKeyID),
				Name:         name,
				RequestCount: raw.RequestCount,
				InputTokens:  raw.InputTokens,
				CachedTokens: raw.CachedTokens,
				OutputTokens: raw.OutputTokens,
				TotalTokens:  raw.TotalTokens,
				Cost:         raw.Cost,
			})
		}
	}

	return results, nil
}

func (r *queryResolver) queryUserStats(ctx context.Context, filter *AnalyticsFilter, apiKeyIDs []int, hasUserFilter bool, loc *time.Location) ([]dimStats, error) {
	type userStatsRaw struct {
		UserID       int     `json:"user_id"`
		FirstName    string  `json:"first_name"`
		LastName     string  `json:"last_name"`
		Email        string  `json:"email"`
		RequestCount int     `json:"request_count"`
		InputTokens  int64   `json:"input_tokens"`
		CachedTokens int64   `json:"cached_tokens"`
		OutputTokens int64   `json:"output_tokens"`
		TotalTokens  int64   `json:"total_tokens"`
		Cost         float64 `json:"cost"`
	}

	var rawResults []userStatsRaw

	err := r.client.UsageLog.Query().
		Where(usagelog.APIKeyIDNotNil()).
		Modify(func(s *sql.Selector) {
			apiKeyTable := sql.Table(apikey.Table)
			userTable := sql.Table("users")

			s.Join(apiKeyTable).On(
				s.C(usagelog.FieldAPIKeyID),
				apiKeyTable.C(apikey.FieldID),
			)
			s.Join(userTable).On(
				apiKeyTable.C(apikey.FieldUserID),
				userTable.C("id"),
			)
			s.Where(sql.EQ(apiKeyTable.C(apikey.FieldDeletedAt), 0))

			r.buildAnalyticsWhere(s, filter, apiKeyIDs, hasUserFilter, loc)

			s.Select(
				sql.As(userTable.C("id"), "user_id"),
				sql.As(userTable.C("first_name"), "first_name"),
				sql.As(userTable.C("last_name"), "last_name"),
				sql.As(userTable.C("email"), "email"),
				sql.As(sql.Count(s.C(usagelog.FieldID)), "request_count"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldPromptTokens)), "input_tokens"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldPromptCachedTokens)), "cached_tokens"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldCompletionTokens)), "output_tokens"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldTotalTokens)), "total_tokens"),
				sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldTotalCost)), "cost"),
			).
				GroupBy(
					userTable.C("id"),
					userTable.C("first_name"),
					userTable.C("last_name"),
					userTable.C("email"),
				).
				OrderBy(sql.Desc("total_tokens"))
		}).
		Scan(ctx, &rawResults)
	if err != nil {
		return nil, fmt.Errorf("failed to get analytics stats by user: %w", err)
	}

	results := make([]dimStats, 0, len(rawResults))

	for _, raw := range rawResults {
		userName := fmt.Sprintf("%s %s", raw.FirstName, raw.LastName)
		userName = trimSpace(userName)
		if userName == "" {
			userName = raw.Email
		}
		results = append(results, dimStats{
			ID:           fmt.Sprintf("%d", raw.UserID),
			Name:         userName,
			RequestCount: raw.RequestCount,
			InputTokens:  raw.InputTokens,
			CachedTokens: raw.CachedTokens,
			OutputTokens: raw.OutputTokens,
			TotalTokens:  raw.TotalTokens,
			Cost:         raw.Cost,
		})
	}

	return results, nil
}

func dimStatsToDimensionStats(items []dimStats) []*AnalyticsDimensionStat {
	return lo.Map(items, func(item dimStats, _ int) *AnalyticsDimensionStat {
		return &AnalyticsDimensionStat{
			ID:                item.ID,
			Name:              item.Name,
			RequestCount:      item.RequestCount,
			InputTokens:       safeIntFromInt64(item.InputTokens),
			CachedInputTokens: safeIntFromInt64(item.CachedTokens),
			OutputTokens:      safeIntFromInt64(item.OutputTokens),
			TotalTokens:       safeIntFromInt64(item.TotalTokens),
			Cost:              item.Cost,
		}
	})
}
