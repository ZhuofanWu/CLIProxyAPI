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
	tokenBreakdownGranularityHour = "hour"
	tokenBreakdownGranularityDay  = "day"

	tokenBreakdownRange7h  = "7h"
	tokenBreakdownRange24h = "24h"
	tokenBreakdownRange7d  = "7d"
	tokenBreakdownRangeAll = "all"

	tokenBreakdownAllPageDays = 30
)

type tokenBreakdownRow struct {
	Key             string
	InputTokens     int64
	OutputTokens    int64
	CachedTokens    int64
	ReasoningTokens int64
}

type tokenBreakdownWindow struct {
	Start       time.Time
	End         time.Time
	BucketCount int
}

func (s *sqliteStore) TokenBreakdownContext(
	ctx context.Context,
	options TokenBreakdownOptions,
) (TokenBreakdownSnapshot, error) {
	options, err := normalizeTokenBreakdownOptions(options)
	if err != nil {
		return TokenBreakdownSnapshot{}, err
	}

	snapshot, window := newTokenBreakdownSnapshot(options)

	db, err := s.openDatabase()
	if err != nil {
		if errors.Is(err, sql.ErrConnDone) {
			return snapshot, nil
		}
		return TokenBreakdownSnapshot{}, err
	}
	defer db.Close()

	rows, err := queryTokenBreakdownRows(ctx, db, options.Granularity, window)
	if err != nil {
		return TokenBreakdownSnapshot{}, err
	}
	rowMap := make(map[string]tokenBreakdownRow, len(rows))
	for _, row := range rows {
		rowMap[row.Key] = row
	}

	for index := range snapshot.Buckets {
		bucketTime := tokenBreakdownBucketTime(window.Start, options.Granularity, index)
		bucketKey := tokenBreakdownBucketKey(options.Granularity, bucketTime)
		if row, ok := rowMap[bucketKey]; ok {
			snapshot.Buckets[index].InputTokens = row.InputTokens
			snapshot.Buckets[index].OutputTokens = row.OutputTokens
			snapshot.Buckets[index].CachedTokens = row.CachedTokens
			snapshot.Buckets[index].ReasoningTokens = row.ReasoningTokens
		}
	}

	if options.Granularity == tokenBreakdownGranularityDay && options.Range == tokenBreakdownRangeAll {
		hasOlder, err := queryTokenBreakdownHasOlder(ctx, db, window.Start)
		if err != nil {
			return TokenBreakdownSnapshot{}, err
		}
		snapshot.HasOlder = hasOlder
	}

	return snapshot, nil
}

func normalizeTokenBreakdownOptions(options TokenBreakdownOptions) (TokenBreakdownOptions, error) {
	options.Granularity = strings.ToLower(strings.TrimSpace(options.Granularity))
	if options.Granularity == "" {
		options.Granularity = tokenBreakdownGranularityHour
	}
	switch options.Granularity {
	case tokenBreakdownGranularityHour, tokenBreakdownGranularityDay:
	default:
		return TokenBreakdownOptions{}, fmt.Errorf("invalid token breakdown granularity: %s", options.Granularity)
	}

	options.Range = strings.ToLower(strings.TrimSpace(options.Range))
	if options.Range == "" {
		options.Range = tokenBreakdownRangeAll
	}
	switch options.Range {
	case tokenBreakdownRange7h, tokenBreakdownRange24h, tokenBreakdownRange7d, tokenBreakdownRangeAll:
	default:
		return TokenBreakdownOptions{}, fmt.Errorf("invalid token breakdown range: %s", options.Range)
	}

	if options.Offset < 0 {
		return TokenBreakdownOptions{}, fmt.Errorf("invalid token breakdown offset: %d", options.Offset)
	}
	if options.Now.IsZero() {
		options.Now = time.Now().UTC()
	} else {
		options.Now = options.Now.UTC()
	}
	if options.Granularity != tokenBreakdownGranularityDay || options.Range != tokenBreakdownRangeAll {
		options.Offset = 0
	}
	return options, nil
}

func newTokenBreakdownSnapshot(options TokenBreakdownOptions) (TokenBreakdownSnapshot, tokenBreakdownWindow) {
	window := resolveTokenBreakdownWindow(options)
	snapshot := TokenBreakdownSnapshot{
		Granularity: options.Granularity,
		Range:       options.Range,
		Offset:      options.Offset,
		HasOlder:    false,
		Buckets:     make([]TokenBreakdownBucket, window.BucketCount),
	}
	for index := 0; index < window.BucketCount; index++ {
		bucketTime := tokenBreakdownBucketTime(window.Start, options.Granularity, index)
		snapshot.Buckets[index] = TokenBreakdownBucket{
			Label: formatTokenBreakdownBucketLabel(options.Granularity, bucketTime),
		}
	}
	return snapshot, window
}

func resolveTokenBreakdownWindow(options TokenBreakdownOptions) tokenBreakdownWindow {
	currentDay := usageChartStartOfDay(options.Now)
	switch options.Granularity {
	case tokenBreakdownGranularityDay:
		switch options.Range {
		case tokenBreakdownRange7h, tokenBreakdownRange24h:
			return tokenBreakdownWindow{
				Start:       currentDay,
				End:         currentDay.AddDate(0, 0, 1),
				BucketCount: 1,
			}
		case tokenBreakdownRange7d:
			start := currentDay.AddDate(0, 0, -6)
			return tokenBreakdownWindow{
				Start:       start,
				End:         currentDay.AddDate(0, 0, 1),
				BucketCount: 7,
			}
		default:
			endDay := currentDay.AddDate(0, 0, -options.Offset)
			start := endDay.AddDate(0, 0, -(tokenBreakdownAllPageDays - 1))
			return tokenBreakdownWindow{
				Start:       start,
				End:         endDay.AddDate(0, 0, 1),
				BucketCount: tokenBreakdownAllPageDays,
			}
		}
	default:
		currentHour := usageChartStartOfHour(options.Now)
		bucketCount := 24
		if options.Range == tokenBreakdownRange7h {
			bucketCount = 7
		}
		start := currentHour.Add(-time.Duration(bucketCount-1) * time.Hour)
		return tokenBreakdownWindow{
			Start:       start,
			End:         currentHour.Add(time.Hour),
			BucketCount: bucketCount,
		}
	}
}

func tokenBreakdownBucketTime(start time.Time, granularity string, offset int) time.Time {
	if granularity == tokenBreakdownGranularityDay {
		return start.AddDate(0, 0, offset)
	}
	return start.Add(time.Duration(offset) * time.Hour)
}

func tokenBreakdownBucketKey(granularity string, bucketTime time.Time) string {
	localTime := usageChartLocalTime(bucketTime)
	if granularity == tokenBreakdownGranularityDay {
		return localTime.Format("2006-01-02")
	}
	return localTime.Format("2006-01-02T15:00:00")
}

func formatTokenBreakdownBucketLabel(granularity string, bucketTime time.Time) string {
	localTime := usageChartLocalTime(bucketTime)
	if granularity == tokenBreakdownGranularityDay {
		return localTime.Format("2006-01-02")
	}
	return localTime.Format("01-02 15:00")
}

func queryTokenBreakdownRows(
	ctx context.Context,
	db *sql.DB,
	granularity string,
	window tokenBreakdownWindow,
) ([]tokenBreakdownRow, error) {
	groupExpr := usageRecordBucketGroupExpr(granularity)

	filter := buildUsageRecordHalfOpenFilter(window.Start, window.End)
	query := fmt.Sprintf(`
		SELECT %s,
		       COALESCE(SUM(input_tokens), 0),
		       COALESCE(SUM(output_tokens), 0),
		       COALESCE(SUM(cached_tokens), 0),
		       COALESCE(SUM(reasoning_tokens), 0)
		FROM usage_records
	`, groupExpr)
	if filter.whereClause != "" {
		query += "\n" + filter.whereClause + "\n"
	}
	query += fmt.Sprintf("GROUP BY %s ORDER BY %s ASC", groupExpr, groupExpr)

	rows, err := db.QueryContext(ctx, query, filter.args...)
	if err != nil {
		return nil, fmt.Errorf("usage statistics: query token breakdown rows: %w", err)
	}
	defer rows.Close()

	result := make([]tokenBreakdownRow, 0, window.BucketCount)
	for rows.Next() {
		var row tokenBreakdownRow
		if err := rows.Scan(
			&row.Key,
			&row.InputTokens,
			&row.OutputTokens,
			&row.CachedTokens,
			&row.ReasoningTokens,
		); err != nil {
			return nil, fmt.Errorf("usage statistics: scan token breakdown row: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usage statistics: iterate token breakdown rows: %w", err)
	}
	return result, nil
}

func queryTokenBreakdownHasOlder(ctx context.Context, db *sql.DB, start time.Time) (bool, error) {
	row := db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM usage_records
			WHERE julianday(timestamp_utc) < julianday(?)
			LIMIT 1
		)
	`, start.UTC().Format(time.RFC3339Nano))
	var exists int
	if err := row.Scan(&exists); err != nil {
		return false, fmt.Errorf("usage statistics: query token breakdown has older: %w", err)
	}
	return exists != 0, nil
}

func buildUsageRecordHalfOpenFilter(start time.Time, end time.Time) usageRecordFilter {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if !start.IsZero() {
		clauses = append(clauses, "julianday(timestamp_utc) >= julianday(?)")
		args = append(args, start.UTC().Format(time.RFC3339Nano))
	}
	if !end.IsZero() {
		clauses = append(clauses, "julianday(timestamp_utc) < julianday(?)")
		args = append(args, end.UTC().Format(time.RFC3339Nano))
	}
	if len(clauses) == 0 {
		return usageRecordFilter{}
	}
	return usageRecordFilter{
		whereClause: "WHERE " + strings.Join(clauses, " AND "),
		args:        args,
	}
}
