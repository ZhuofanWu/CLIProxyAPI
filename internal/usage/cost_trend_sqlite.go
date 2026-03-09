package usage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type costTrendRow struct {
	Key          string
	ModelName    string
	InputTokens  int64
	OutputTokens int64
	CachedTokens int64
}

func (s *sqliteStore) CostTrendContext(
	ctx context.Context,
	options CostTrendOptions,
) (CostTrendSnapshot, error) {
	tokenOptions, err := normalizeTokenBreakdownOptions(TokenBreakdownOptions{
		Granularity: options.Granularity,
		Range:       options.Range,
		Offset:      options.Offset,
		Now:         options.Now,
	})
	if err != nil {
		return CostTrendSnapshot{}, err
	}
	if options.ModelPrices == nil {
		options.ModelPrices = map[string]ModelPrice{}
	}

	snapshot, window := newCostTrendSnapshot(tokenOptions)

	db, err := s.openDatabase()
	if err != nil {
		if errors.Is(err, sql.ErrConnDone) {
			return snapshot, nil
		}
		return CostTrendSnapshot{}, err
	}
	defer db.Close()

	rows, err := queryCostTrendRows(ctx, db, tokenOptions.Granularity, window)
	if err != nil {
		return CostTrendSnapshot{}, err
	}

	costByKey := make(map[string]float64, len(rows))
	for _, row := range rows {
		costByKey[row.Key] += calculateGeneralCost(
			row.ModelName,
			row.InputTokens,
			row.OutputTokens,
			row.CachedTokens,
			options.ModelPrices,
		)
	}

	for index := range snapshot.Buckets {
		bucketTime := tokenBreakdownBucketTime(window.Start, tokenOptions.Granularity, index)
		bucketKey := tokenBreakdownBucketKey(tokenOptions.Granularity, bucketTime)
		snapshot.Buckets[index].Cost = costByKey[bucketKey]
	}

	if tokenOptions.Granularity == tokenBreakdownGranularityDay &&
		tokenOptions.Range == tokenBreakdownRangeAll {
		hasOlder, err := queryTokenBreakdownHasOlder(ctx, db, window.Start)
		if err != nil {
			return CostTrendSnapshot{}, err
		}
		snapshot.HasOlder = hasOlder
	}

	return snapshot, nil
}

func newCostTrendSnapshot(options TokenBreakdownOptions) (CostTrendSnapshot, tokenBreakdownWindow) {
	window := resolveTokenBreakdownWindow(options)
	snapshot := CostTrendSnapshot{
		Granularity: options.Granularity,
		Range:       options.Range,
		Offset:      options.Offset,
		HasOlder:    false,
		Buckets:     make([]CostTrendBucket, window.BucketCount),
	}
	for index := 0; index < window.BucketCount; index++ {
		bucketTime := tokenBreakdownBucketTime(window.Start, options.Granularity, index)
		snapshot.Buckets[index] = CostTrendBucket{
			Label: formatTokenBreakdownBucketLabel(options.Granularity, bucketTime),
			Cost:  0,
		}
	}
	return snapshot, window
}

func queryCostTrendRows(
	ctx context.Context,
	db *sql.DB,
	granularity string,
	window tokenBreakdownWindow,
) ([]costTrendRow, error) {
	groupExpr := "date(timestamp_utc)"
	if granularity == tokenBreakdownGranularityHour {
		groupExpr = "strftime('%Y-%m-%dT%H:00:00Z', timestamp_utc)"
	}

	filter := buildUsageRecordHalfOpenFilter(window.Start, window.End)
	query := fmt.Sprintf(`
		SELECT %s,
		       model_name,
		       COALESCE(SUM(input_tokens), 0),
		       COALESCE(SUM(output_tokens), 0),
		       COALESCE(SUM(cached_tokens), 0)
		FROM usage_records
	`, groupExpr)
	if filter.whereClause != "" {
		query += "\n" + filter.whereClause + "\n"
	}
	query += fmt.Sprintf("GROUP BY %s, model_name ORDER BY %s ASC, model_name ASC", groupExpr, groupExpr)

	rows, err := db.QueryContext(ctx, query, filter.args...)
	if err != nil {
		return nil, fmt.Errorf("usage statistics: query cost trend rows: %w", err)
	}
	defer rows.Close()

	result := make([]costTrendRow, 0, window.BucketCount)
	for rows.Next() {
		var row costTrendRow
		if err := rows.Scan(
			&row.Key,
			&row.ModelName,
			&row.InputTokens,
			&row.OutputTokens,
			&row.CachedTokens,
		); err != nil {
			return nil, fmt.Errorf("usage statistics: scan cost trend row: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usage statistics: iterate cost trend rows: %w", err)
	}
	return result, nil
}
