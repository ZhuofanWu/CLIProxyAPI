package usage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	trendGranularityHour = "hour"
	trendGranularityDay  = "day"

	trendRange7h  = "7h"
	trendRange24h = "24h"
	trendRange7d  = "7d"
	trendRangeAll = "all"

	trendMetricRequests = "requests"
	trendMetricTokens   = "tokens"
	trendAllModelName   = "all"
	trendAllPageDays    = 15
)

type trendAggregateRow struct {
	BucketKey string
	ModelName string
	Requests  int64
	Tokens    int64
}

type trendWindow struct {
	Start       time.Time
	End         time.Time
	BucketCount int
	Labels      []string
	Keys        []string
}

type trendSeriesValues struct {
	Requests []int64
	Tokens   []int64
}

func (s *sqliteStore) TrendContext(ctx context.Context, options TrendOptions) (TrendSnapshot, error) {
	db, err := s.openDatabase()
	if err != nil {
		if errors.Is(err, sql.ErrConnDone) {
			return TrendSnapshot{}, nil
		}
		return TrendSnapshot{}, err
	}
	defer db.Close()

	normalizedOptions, err := normalizeTrendOptions(options)
	if err != nil {
		return TrendSnapshot{}, err
	}

	window := resolveTrendWindow(normalizedOptions)

	snapshot := TrendSnapshot{
		Granularity: normalizedOptions.Granularity,
		Range:       normalizedOptions.Range,
		Offset:      normalizedOptions.Offset,
		HasOlder:    false,
		Labels:      append([]string(nil), window.Labels...),
		Series:      make([]TrendSeries, 0, len(normalizedOptions.Models)),
	}
	if len(window.Keys) == 0 {
		for _, modelName := range normalizedOptions.Models {
			snapshot.Series = append(snapshot.Series, TrendSeries{
				ModelName: modelName,
				IsAll:     modelName == trendAllModelName,
				Requests:  []int64{},
				Tokens:    []int64{},
			})
		}
		return snapshot, nil
	}

	rows, err := queryTrendRows(ctx, db, normalizedOptions, window)
	if err != nil {
		return TrendSnapshot{}, err
	}

	keyIndex := make(map[string]int, len(window.Keys))
	for index, key := range window.Keys {
		keyIndex[key] = index
	}

	valuesByModel := make(map[string]*trendSeriesValues, len(normalizedOptions.Models))
	totalRequests := make([]int64, len(window.Keys))
	totalTokens := make([]int64, len(window.Keys))

	for _, row := range rows {
		bucketIndex, ok := keyIndex[row.BucketKey]
		if !ok {
			continue
		}

		totalRequests[bucketIndex] += row.Requests
		totalTokens[bucketIndex] += row.Tokens

		if _, wanted := normalizedOptions.modelSet[row.ModelName]; !wanted {
			continue
		}
		values := valuesByModel[row.ModelName]
		if values == nil {
			values = &trendSeriesValues{
				Requests: make([]int64, len(window.Keys)),
				Tokens:   make([]int64, len(window.Keys)),
			}
			valuesByModel[row.ModelName] = values
		}
		values.Requests[bucketIndex] += row.Requests
		values.Tokens[bucketIndex] += row.Tokens
	}

	for _, modelName := range normalizedOptions.Models {
		if modelName == trendAllModelName {
			snapshot.Series = append(snapshot.Series, TrendSeries{
				ModelName: trendAllModelName,
				IsAll:     true,
				Requests:  append([]int64(nil), totalRequests...),
				Tokens:    append([]int64(nil), totalTokens...),
			})
			continue
		}

		values := valuesByModel[modelName]
		if values == nil {
			snapshot.Series = append(snapshot.Series, TrendSeries{
				ModelName: modelName,
				IsAll:     false,
				Requests:  make([]int64, len(window.Keys)),
				Tokens:    make([]int64, len(window.Keys)),
			})
			continue
		}

		snapshot.Series = append(snapshot.Series, TrendSeries{
			ModelName: modelName,
			IsAll:     false,
			Requests:  append([]int64(nil), values.Requests...),
			Tokens:    append([]int64(nil), values.Tokens...),
		})
	}

	if normalizedOptions.Granularity == trendGranularityDay && normalizedOptions.Range == trendRangeAll {
		hasOlder, err := queryTokenBreakdownHasOlder(ctx, db, window.Start)
		if err != nil {
			return TrendSnapshot{}, err
		}
		snapshot.HasOlder = hasOlder
	}

	return snapshot, nil
}

func (s *sqliteStore) TrendModelsContext(
	ctx context.Context,
	options TrendModelsOptions,
) (TrendModelsSnapshot, error) {
	db, err := s.openDatabase()
	if err != nil {
		if errors.Is(err, sql.ErrConnDone) {
			return TrendModelsSnapshot{}, nil
		}
		return TrendModelsSnapshot{}, err
	}
	defer db.Close()

	options = normalizeTrendModelsOptions(options)
	models, err := queryTrendModelItems(ctx, db, options)
	if err != nil {
		return TrendModelsSnapshot{}, err
	}

	return TrendModelsSnapshot{
		Range:  options.Range,
		Models: models,
	}, nil
}

type normalizedTrendOptions struct {
	Granularity string
	Range       string
	Offset      int
	Now         time.Time
	Models      []string
	modelSet    map[string]struct{}
}

func normalizeTrendOptions(options TrendOptions) (normalizedTrendOptions, error) {
	granularity := strings.ToLower(strings.TrimSpace(options.Granularity))
	if granularity == "" {
		granularity = trendGranularityDay
	}
	switch granularity {
	case trendGranularityHour, trendGranularityDay:
	default:
		return normalizedTrendOptions{}, fmt.Errorf("invalid usage trend granularity: %s", options.Granularity)
	}

	queryRange := strings.ToLower(strings.TrimSpace(options.Range))
	if queryRange == "" {
		queryRange = trendRangeAll
	}
	switch queryRange {
	case trendRange7h, trendRange24h, trendRange7d, trendRangeAll:
	default:
		return normalizedTrendOptions{}, fmt.Errorf("invalid usage trend range: %s", options.Range)
	}

	now := options.Now
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	offset := options.Offset
	if offset < 0 {
		return normalizedTrendOptions{}, fmt.Errorf("invalid usage trend offset: %d", offset)
	}

	models := make([]string, 0, len(options.Models))
	modelSet := make(map[string]struct{}, len(options.Models))
	for _, rawModel := range options.Models {
		modelName := strings.TrimSpace(rawModel)
		if modelName == "" {
			continue
		}
		if strings.EqualFold(modelName, trendAllModelName) {
			modelName = trendAllModelName
		}
		if _, exists := modelSet[modelName]; exists {
			continue
		}
		models = append(models, modelName)
		modelSet[modelName] = struct{}{}
	}
	if len(models) == 0 {
		models = []string{trendAllModelName}
		modelSet[trendAllModelName] = struct{}{}
	}
	if granularity != trendGranularityDay || queryRange != trendRangeAll {
		offset = 0
	}

	return normalizedTrendOptions{
		Granularity: granularity,
		Range:       queryRange,
		Offset:      offset,
		Now:         now,
		Models:      models,
		modelSet:    modelSet,
	}, nil
}

func normalizeTrendModelsOptions(options TrendModelsOptions) TrendModelsOptions {
	if !options.Since.IsZero() {
		options.Since = options.Since.UTC()
	}
	if options.Now.IsZero() {
		options.Now = time.Now().UTC()
	} else {
		options.Now = options.Now.UTC()
	}
	options.Range = strings.ToLower(strings.TrimSpace(options.Range))
	if options.Range == "" {
		options.Range = trendRangeAll
	}
	return options
}

func resolveTrendWindow(options normalizedTrendOptions) trendWindow {
	if options.Granularity == trendGranularityHour {
		bucketCount := 24
		if options.Range == trendRange7h {
			bucketCount = 7
		}

		currentHour := usageChartStartOfHour(options.Now)
		start := currentHour.Add(-time.Duration(bucketCount-1) * time.Hour)
		end := currentHour.Add(time.Hour)
		return buildTrendWindow(start, end, bucketCount, trendGranularityHour)
	}

	currentDay := usageChartStartOfDay(options.Now)

	switch options.Range {
	case trendRange7h, trendRange24h:
		return buildTrendWindow(currentDay, currentDay.AddDate(0, 0, 1), 1, trendGranularityDay)
	case trendRange7d:
		startDay := currentDay.AddDate(0, 0, -6)
		return buildTrendWindow(startDay, currentDay.AddDate(0, 0, 1), 7, trendGranularityDay)
	case trendRangeAll:
		endDay := currentDay.AddDate(0, 0, -options.Offset)
		startDay := endDay.AddDate(0, 0, -(trendAllPageDays - 1))
		return buildTrendWindow(startDay, endDay.AddDate(0, 0, 1), trendAllPageDays, trendGranularityDay)
	default:
		return buildTrendWindow(currentDay, currentDay.AddDate(0, 0, 1), 1, trendGranularityDay)
	}
}

func buildTrendWindow(start time.Time, end time.Time, bucketCount int, granularity string) trendWindow {
	if bucketCount <= 0 {
		return trendWindow{}
	}
	window := trendWindow{
		Start:       start.UTC(),
		End:         end.UTC(),
		BucketCount: bucketCount,
		Labels:      make([]string, bucketCount),
		Keys:        make([]string, bucketCount),
	}
	for index := 0; index < bucketCount; index++ {
		bucketTime := tokenBreakdownBucketTime(window.Start, granularity, index)
		window.Labels[index] = formatTokenBreakdownBucketLabel(granularity, bucketTime)
		window.Keys[index] = tokenBreakdownBucketKey(granularity, bucketTime)
	}
	return window
}

func queryTrendRows(
	ctx context.Context,
	db *sql.DB,
	options normalizedTrendOptions,
	window trendWindow,
) ([]trendAggregateRow, error) {
	groupExpr := usageRecordBucketGroupExpr(options.Granularity)

	windowFilter := buildUsageRecordHalfOpenFilter(window.Start, window.End)
	clauses := make([]string, 0, 2)
	args := make([]any, 0, len(windowFilter.args)+len(options.Models))
	if windowFilter.whereClause != "" {
		clauses = append(clauses, strings.TrimSpace(strings.TrimPrefix(windowFilter.whereClause, "WHERE")))
		args = append(args, windowFilter.args...)
	}

	if _, hasAll := options.modelSet[trendAllModelName]; !hasAll && len(options.Models) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(options.Models)), ",")
		clauses = append(clauses, "model_name IN ("+placeholders+")")
		for _, modelName := range options.Models {
			args = append(args, modelName)
		}
	}

	query := fmt.Sprintf(`
		SELECT %s,
		       model_name,
		       COUNT(*),
		       COALESCE(SUM(total_tokens), 0)
		FROM usage_records
	`, groupExpr)
	if len(clauses) > 0 {
		query += "\nWHERE " + strings.Join(clauses, " AND ") + "\n"
	}
	query += fmt.Sprintf("GROUP BY %s, model_name ORDER BY %s ASC, model_name ASC", groupExpr, groupExpr)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("usage statistics: query usage trend rows: %w", err)
	}
	defer rows.Close()

	result := make([]trendAggregateRow, 0, len(options.Models)*window.BucketCount)
	for rows.Next() {
		var row trendAggregateRow
		if err := rows.Scan(&row.BucketKey, &row.ModelName, &row.Requests, &row.Tokens); err != nil {
			return nil, fmt.Errorf("usage statistics: scan usage trend row: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usage statistics: iterate usage trend rows: %w", err)
	}
	return result, nil
}

func queryTrendModelItems(
	ctx context.Context,
	db *sql.DB,
	options TrendModelsOptions,
) ([]TrendModelItem, error) {
	filter := buildUsageRecordWindowFilter(options.Since, options.Now)
	query := `
		SELECT model_name,
		       COUNT(*),
		       COALESCE(SUM(total_tokens), 0)
		FROM usage_records
	`
	if filter.whereClause != "" {
		query += "\n" + filter.whereClause + "\n"
	}
	query += "GROUP BY model_name ORDER BY COUNT(*) DESC, COALESCE(SUM(total_tokens), 0) DESC, model_name ASC"

	rows, err := db.QueryContext(ctx, query, filter.args...)
	if err != nil {
		return nil, fmt.Errorf("usage statistics: query usage trend models: %w", err)
	}
	defer rows.Close()

	result := make([]TrendModelItem, 0, 16)
	for rows.Next() {
		var item TrendModelItem
		if err := rows.Scan(&item.ModelName, &item.Requests, &item.Tokens); err != nil {
			return nil, fmt.Errorf("usage statistics: scan usage trend model: %w", err)
		}
		item.ModelName = strings.TrimSpace(item.ModelName)
		if item.ModelName == "" {
			item.ModelName = "unknown"
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usage statistics: iterate usage trend models: %w", err)
	}
	return result, nil
}

func buildMetricTrendSnapshot(snapshot TrendSnapshot, metric string) MetricTrendSnapshot {
	result := MetricTrendSnapshot{
		Metric:      metric,
		Granularity: snapshot.Granularity,
		Range:       snapshot.Range,
		Offset:      snapshot.Offset,
		HasOlder:    snapshot.HasOlder,
		Labels:      append([]string(nil), snapshot.Labels...),
		Series:      make([]MetricTrendSeries, 0, len(snapshot.Series)),
	}

	for _, series := range snapshot.Series {
		projected := MetricTrendSeries{
			ModelName: series.ModelName,
			IsAll:     series.IsAll,
		}
		if metric == trendMetricTokens {
			projected.Values = append([]int64(nil), series.Tokens...)
		} else {
			projected.Values = append([]int64(nil), series.Requests...)
		}
		result.Series = append(result.Series, projected)
	}

	return result
}
