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

	window, err := resolveTrendWindow(ctx, db, normalizedOptions)
	if err != nil {
		return TrendSnapshot{}, err
	}

	snapshot := TrendSnapshot{
		Granularity: normalizedOptions.Granularity,
		Range:       normalizedOptions.Range,
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

	return normalizedTrendOptions{
		Granularity: granularity,
		Range:       queryRange,
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

func resolveTrendWindow(
	ctx context.Context,
	db *sql.DB,
	options normalizedTrendOptions,
) (trendWindow, error) {
	if options.Granularity == trendGranularityHour {
		bucketCount := 24
		switch options.Range {
		case trendRange7h:
			bucketCount = 7
		case trendRange7d:
			bucketCount = 7 * 24
		}

		currentHour := options.Now.Truncate(time.Hour)
		start := currentHour.Add(-time.Duration(bucketCount-1) * time.Hour)
		end := currentHour.Add(time.Hour)
		return buildTrendWindow(start, end, bucketCount, trendGranularityHour), nil
	}

	currentDay := time.Date(
		options.Now.Year(),
		options.Now.Month(),
		options.Now.Day(),
		0,
		0,
		0,
		0,
		time.UTC,
	)

	var startDay time.Time
	switch options.Range {
	case trendRangeAll:
		earliest, ok, err := queryTrendEarliestTimestamp(ctx, db)
		if err != nil {
			return trendWindow{}, err
		}
		if !ok {
			return trendWindow{}, nil
		}
		startDay = time.Date(
			earliest.Year(),
			earliest.Month(),
			earliest.Day(),
			0,
			0,
			0,
			0,
			time.UTC,
		)
	default:
		since := options.Now.Add(-trendRangeDuration(options.Range))
		startDay = time.Date(
			since.Year(),
			since.Month(),
			since.Day(),
			0,
			0,
			0,
			0,
			time.UTC,
		)
	}

	bucketCount := int(currentDay.Sub(startDay)/(24*time.Hour)) + 1
	if bucketCount < 1 {
		bucketCount = 1
	}

	return buildTrendWindow(startDay, currentDay.AddDate(0, 0, 1), bucketCount, trendGranularityDay), nil
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

func trendRangeDuration(queryRange string) time.Duration {
	switch queryRange {
	case trendRange7h:
		return 7 * time.Hour
	case trendRange24h:
		return 24 * time.Hour
	case trendRange7d:
		return 7 * 24 * time.Hour
	default:
		return 0
	}
}

func queryTrendRows(
	ctx context.Context,
	db *sql.DB,
	options normalizedTrendOptions,
	window trendWindow,
) ([]trendAggregateRow, error) {
	groupExpr := "date(timestamp_utc)"
	if options.Granularity == trendGranularityHour {
		groupExpr = "strftime('%Y-%m-%dT%H:00:00Z', timestamp_utc)"
	}

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

func queryTrendEarliestTimestamp(ctx context.Context, db *sql.DB) (time.Time, bool, error) {
	row := db.QueryRowContext(ctx, `
		SELECT timestamp_utc
		FROM usage_records
		ORDER BY julianday(timestamp_utc) ASC
		LIMIT 1
	`)

	var timestampUTC sql.NullString
	if err := row.Scan(&timestampUTC); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, fmt.Errorf("usage statistics: query earliest trend timestamp: %w", err)
	}
	if !timestampUTC.Valid || strings.TrimSpace(timestampUTC.String) == "" {
		return time.Time{}, false, nil
	}

	parsedTime, err := time.Parse(time.RFC3339Nano, timestampUTC.String)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("usage statistics: parse earliest trend timestamp: %w", err)
	}
	return parsedTime.UTC(), true, nil
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
