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
	usageGeneralRequestsWindowMinutes = 60
	usageGeneralRatesWindowMinutes    = 30
	usagePriceUnitDivisor             = 1_000_000_000.0
)

type generalRecord struct {
	ModelName    string
	Timestamp    time.Time
	InputTokens  int64
	OutputTokens int64
	CachedTokens int64
	TotalTokens  int64
}

func (s *sqliteStore) GeneralContext(ctx context.Context, options GeneralOptions) (GeneralSnapshot, error) {
	db, err := s.openDatabase()
	if err != nil {
		if errors.Is(err, sql.ErrConnDone) {
			return GeneralSnapshot{}, nil
		}
		return GeneralSnapshot{}, err
	}
	defer db.Close()

	options = normalizeGeneralOptions(options)
	result := GeneralSnapshot{
		Summary: GeneralSummary{CostAvailable: len(options.ModelPrices) > 0},
	}

	summary, err := queryGeneralSummary(ctx, db, options)
	if err != nil {
		return GeneralSnapshot{}, err
	}
	result.Summary.TotalRequests = summary.TotalRequests
	result.Summary.SuccessCount = summary.SuccessCount
	result.Summary.FailureCount = summary.FailureCount
	result.Summary.TotalTokens = summary.TotalTokens
	result.Summary.CachedTokens = summary.CachedTokens
	result.Summary.ReasoningTokens = summary.ReasoningTokens

	if result.Summary.CostAvailable {
		totalCost, err := queryGeneralTotalCost(ctx, db, options)
		if err != nil {
			return GeneralSnapshot{}, err
		}
		result.Summary.TotalCost = totalCost
	}

	recentWindowStart := options.Now.Add(-usageGeneralRequestsWindowMinutes * time.Minute)
	recentRecords, err := queryGeneralRecentRecords(
		ctx,
		db,
		maxTime(options.Since, recentWindowStart),
		options.Now,
	)
	if err != nil {
		return GeneralSnapshot{}, err
	}

	result.Series.Requests60m, result.Series.Tokens60m = buildRequestAndTokenSeries(
		recentRecords,
		options.Now,
		usageGeneralRequestsWindowMinutes,
	)

	rateWindowStart := options.Now.Add(-usageGeneralRatesWindowMinutes * time.Minute)
	recentRateRecords := filterGeneralRecordsSince(recentRecords, rateWindowStart)
	result.Series.RPM30m, result.Series.TPM30m, result.Summary.RPMRequestCount30m, result.Summary.TPMTokenCount30m =
		buildRateSeries(recentRateRecords, options.Now, usageGeneralRatesWindowMinutes)
	result.Summary.RPM30m = float64(result.Summary.RPMRequestCount30m) / usageGeneralRatesWindowMinutes
	result.Summary.TPM30m = float64(result.Summary.TPMTokenCount30m) / usageGeneralRatesWindowMinutes
	result.Series.Cost30m = buildCostSeries(
		recentRateRecords,
		options.Now,
		usageGeneralRatesWindowMinutes,
		options.ModelPrices,
	)

	return result, nil
}

func normalizeGeneralOptions(options GeneralOptions) GeneralOptions {
	if options.Now.IsZero() {
		options.Now = time.Now().UTC()
	} else {
		options.Now = options.Now.UTC()
	}
	if !options.Since.IsZero() {
		options.Since = options.Since.UTC()
	}
	if options.ModelPrices == nil {
		options.ModelPrices = map[string]ModelPrice{}
	}
	return options
}

type generalSummaryRow struct {
	TotalRequests   int64
	SuccessCount    int64
	FailureCount    int64
	TotalTokens     int64
	CachedTokens    int64
	ReasoningTokens int64
}

func queryGeneralSummary(ctx context.Context, db *sql.DB, options GeneralOptions) (generalSummaryRow, error) {
	filter := buildUsageRecordWindowFilter(options.Since, options.Now)
	query := `
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN failed = 0 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN failed != 0 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(total_tokens), 0),
		       COALESCE(SUM(cached_tokens), 0),
		       COALESCE(SUM(reasoning_tokens), 0)
		FROM usage_records
	`
	if filter.whereClause != "" {
		query += "\n" + filter.whereClause + "\n"
	}
	row := db.QueryRowContext(ctx, query, filter.args...)
	var result generalSummaryRow
	if err := row.Scan(
		&result.TotalRequests,
		&result.SuccessCount,
		&result.FailureCount,
		&result.TotalTokens,
		&result.CachedTokens,
		&result.ReasoningTokens,
	); err != nil {
		return generalSummaryRow{}, fmt.Errorf("usage statistics: query usage general summary: %w", err)
	}
	return result, nil
}

func queryGeneralTotalCost(ctx context.Context, db *sql.DB, options GeneralOptions) (float64, error) {
	if len(options.ModelPrices) == 0 {
		return 0, nil
	}
	filter := buildUsageRecordWindowFilter(options.Since, options.Now)
	query := `
		SELECT model_name,
		       COALESCE(SUM(input_tokens), 0),
		       COALESCE(SUM(output_tokens), 0),
		       COALESCE(SUM(cached_tokens), 0)
		FROM usage_records
	`
	if filter.whereClause != "" {
		query += "\n" + filter.whereClause + "\n"
	}
	query += "GROUP BY model_name ORDER BY model_name ASC"
	rows, err := db.QueryContext(ctx, query, filter.args...)
	if err != nil {
		return 0, fmt.Errorf("usage statistics: query usage general cost: %w", err)
	}
	defer rows.Close()

	totalCost := 0.0
	for rows.Next() {
		var (
			modelName    string
			inputTokens  int64
			outputTokens int64
			cachedTokens int64
		)
		if err := rows.Scan(&modelName, &inputTokens, &outputTokens, &cachedTokens); err != nil {
			return 0, fmt.Errorf("usage statistics: scan usage general cost: %w", err)
		}
		totalCost += calculateGeneralCost(modelName, inputTokens, outputTokens, cachedTokens, options.ModelPrices)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("usage statistics: iterate usage general cost: %w", err)
	}
	return totalCost, nil
}

func queryGeneralRecentRecords(
	ctx context.Context,
	db *sql.DB,
	start time.Time,
	end time.Time,
) ([]generalRecord, error) {
	filter := buildUsageRecordWindowFilter(start, end)
	query := `
		SELECT model_name, timestamp_utc, input_tokens, output_tokens, cached_tokens, total_tokens
		FROM usage_records
	`
	if filter.whereClause != "" {
		query += "\n" + filter.whereClause + "\n"
	}
	query += "ORDER BY julianday(timestamp_utc) ASC"
	rows, err := db.QueryContext(ctx, query, filter.args...)
	if err != nil {
		return nil, fmt.Errorf("usage statistics: query usage general recent records: %w", err)
	}
	defer rows.Close()

	records := make([]generalRecord, 0, 128)
	for rows.Next() {
		var (
			record       generalRecord
			timestampUTC string
		)
		if err := rows.Scan(
			&record.ModelName,
			&timestampUTC,
			&record.InputTokens,
			&record.OutputTokens,
			&record.CachedTokens,
			&record.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("usage statistics: scan usage general recent record: %w", err)
		}
		parsedTime, err := time.Parse(time.RFC3339Nano, timestampUTC)
		if err != nil {
			return nil, fmt.Errorf("usage statistics: parse usage general recent record timestamp: %w", err)
		}
		record.Timestamp = parsedTime.UTC()
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usage statistics: iterate usage general recent records: %w", err)
	}
	return records, nil
}

func buildUsageRecordWindowFilter(start time.Time, end time.Time) usageRecordFilter {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if !start.IsZero() {
		clauses = append(clauses, "julianday(timestamp_utc) >= julianday(?)")
		args = append(args, start.UTC().Format(time.RFC3339Nano))
	}
	if !end.IsZero() {
		clauses = append(clauses, "julianday(timestamp_utc) <= julianday(?)")
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

func buildRequestAndTokenSeries(
	records []generalRecord,
	now time.Time,
	windowMinutes int,
) ([]GeneralPoint, []GeneralPoint) {
	requests := buildGeneralSeriesSkeleton(now, windowMinutes)
	tokens := buildGeneralSeriesSkeleton(now, windowMinutes)
	windowStart := now.Add(-time.Duration(windowMinutes) * time.Minute)
	for _, record := range records {
		bucketIndex := generalBucketIndex(record.Timestamp, windowStart, now, windowMinutes)
		if bucketIndex < 0 {
			continue
		}
		requests[bucketIndex].Value += 1
		tokens[bucketIndex].Value += float64(record.TotalTokens)
	}
	return requests, tokens
}

func buildRateSeries(
	records []generalRecord,
	now time.Time,
	windowMinutes int,
) ([]GeneralPoint, []GeneralPoint, int64, int64) {
	rpm := buildGeneralSeriesSkeleton(now, windowMinutes)
	tpm := buildGeneralSeriesSkeleton(now, windowMinutes)
	windowStart := now.Add(-time.Duration(windowMinutes) * time.Minute)
	var requestCount int64
	var tokenCount int64
	for _, record := range records {
		bucketIndex := generalBucketIndex(record.Timestamp, windowStart, now, windowMinutes)
		if bucketIndex < 0 {
			continue
		}
		rpm[bucketIndex].Value += 1
		tpm[bucketIndex].Value += float64(record.TotalTokens)
		requestCount++
		tokenCount += record.TotalTokens
	}
	return rpm, tpm, requestCount, tokenCount
}

func buildCostSeries(
	records []generalRecord,
	now time.Time,
	windowMinutes int,
	modelPrices map[string]ModelPrice,
) []GeneralPoint {
	series := buildGeneralSeriesSkeleton(now, windowMinutes)
	if len(modelPrices) == 0 {
		return series
	}
	windowStart := now.Add(-time.Duration(windowMinutes) * time.Minute)
	for _, record := range records {
		bucketIndex := generalBucketIndex(record.Timestamp, windowStart, now, windowMinutes)
		if bucketIndex < 0 {
			continue
		}
		series[bucketIndex].Value += calculateGeneralCost(
			record.ModelName,
			record.InputTokens,
			record.OutputTokens,
			record.CachedTokens,
			modelPrices,
		)
	}
	return series
}

func buildGeneralSeriesSkeleton(now time.Time, windowMinutes int) []GeneralPoint {
	points := make([]GeneralPoint, windowMinutes)
	windowStart := now.Add(-time.Duration(windowMinutes) * time.Minute)
	for index := range points {
		points[index] = GeneralPoint{
			Timestamp: windowStart.Add(time.Duration(index+1) * time.Minute).UTC(),
			Value:     0,
		}
	}
	return points
}

func generalBucketIndex(timestamp time.Time, windowStart time.Time, now time.Time, windowMinutes int) int {
	if timestamp.Before(windowStart) || timestamp.After(now) {
		return -1
	}
	bucketIndex := int(timestamp.Sub(windowStart) / time.Minute)
	if bucketIndex < 0 {
		return -1
	}
	if bucketIndex >= windowMinutes {
		return windowMinutes - 1
	}
	return bucketIndex
}

func filterGeneralRecordsSince(records []generalRecord, since time.Time) []generalRecord {
	if since.IsZero() {
		return records
	}
	filtered := make([]generalRecord, 0, len(records))
	for _, record := range records {
		if record.Timestamp.Before(since) {
			continue
		}
		filtered = append(filtered, record)
	}
	return filtered
}

func calculateGeneralCost(
	modelName string,
	inputTokens int64,
	outputTokens int64,
	cachedTokens int64,
	modelPrices map[string]ModelPrice,
) float64 {
	price, ok := modelPrices[modelName]
	if !ok {
		return 0
	}
	promptTokens := inputTokens - cachedTokens
	if promptTokens < 0 {
		promptTokens = 0
	}
	totalMilli :=
		promptTokens*price.PromptMilli +
			cachedTokens*price.CacheMilli +
			outputTokens*price.CompletionMilli
	if totalMilli <= 0 {
		return 0
	}
	return float64(totalMilli) / usagePriceUnitDivisor
}

func maxTime(left time.Time, right time.Time) time.Time {
	if left.IsZero() {
		return right.UTC()
	}
	if right.IsZero() {
		return left.UTC()
	}
	if left.After(right) {
		return left.UTC()
	}
	return right.UTC()
}
