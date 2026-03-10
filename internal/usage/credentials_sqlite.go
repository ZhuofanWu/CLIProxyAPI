package usage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	usageCredentialHealthRows          = 1
	usageCredentialHealthCols          = 20
	usageCredentialHealthBucketMinutes = 15
	usageCredentialHealthBucketCount   = usageCredentialHealthRows * usageCredentialHealthCols
)

var usageCredentialHealthWindowDuration = time.Duration(usageCredentialHealthBucketCount*usageCredentialHealthBucketMinutes) * time.Minute

type credentialAggregateRow struct {
	Source    string
	AuthIndex string
	Success   int64
	Failure   int64
}

type credentialHealthBucketRow struct {
	Source       string
	AuthIndex    string
	Index        int
	SuccessCount int64
	FailureCount int64
}

func (s *sqliteStore) CredentialsContext(
	ctx context.Context,
	options CredentialsOptions,
) (CredentialUsageSnapshot, error) {
	db, err := s.openDatabase()
	if err != nil {
		if errors.Is(err, sql.ErrConnDone) {
			return CredentialUsageSnapshot{}, nil
		}
		return CredentialUsageSnapshot{}, err
	}
	defer db.Close()

	options = normalizeCredentialsOptions(options)
	aggregateRows, err := queryCredentialAggregateRows(ctx, db, options)
	if err != nil {
		return CredentialUsageSnapshot{}, err
	}

	result := CredentialUsageSnapshot{
		Range:       options.Range,
		PercentData: options.IncludePercentData,
		Credentials: make([]CredentialUsageItem, 0, len(aggregateRows)),
	}
	itemByKey := make(map[string]*CredentialUsageItem, len(aggregateRows))
	for _, row := range aggregateRows {
		total := row.Success + row.Failure
		item := CredentialUsageItem{
			Source:      row.Source,
			AuthIndex:   row.AuthIndex,
			Success:     row.Success,
			Failure:     row.Failure,
			Total:       total,
			SuccessRate: 100,
		}
		if total > 0 {
			item.SuccessRate = float64(row.Success) * 100 / float64(total)
		}
		if options.IncludePercentData {
			health := newCredentialHealthSnapshot(options.healthWindowStart(), options.Now)
			item.Health = &health
		}

		result.Credentials = append(result.Credentials, item)
		resultIndex := len(result.Credentials) - 1
		itemByKey[credentialUsageKey(row.Source, row.AuthIndex)] = &result.Credentials[resultIndex]
	}

	if !options.IncludePercentData || len(result.Credentials) == 0 {
		return result, nil
	}

	buckets, err := queryCredentialHealthBuckets(ctx, db, options)
	if err != nil {
		return CredentialUsageSnapshot{}, err
	}
	for _, bucket := range buckets {
		item := itemByKey[credentialUsageKey(bucket.Source, bucket.AuthIndex)]
		if item == nil || item.Health == nil {
			continue
		}
		if bucket.Index < 0 || bucket.Index >= usageCredentialHealthBucketCount {
			continue
		}
		item.Health.SuccessCounts[bucket.Index] = bucket.SuccessCount
		item.Health.FailureCounts[bucket.Index] = bucket.FailureCount
		total := bucket.SuccessCount + bucket.FailureCount
		if total > 0 {
			item.Health.Rates[bucket.Index] = int(math.Round(float64(bucket.SuccessCount) * 100 / float64(total)))
		}
		item.Health.TotalSuccess += bucket.SuccessCount
		item.Health.TotalFailure += bucket.FailureCount
	}

	for index := range result.Credentials {
		health := result.Credentials[index].Health
		if health == nil {
			continue
		}
		total := health.TotalSuccess + health.TotalFailure
		if total > 0 {
			health.SuccessRate = float64(health.TotalSuccess) * 100 / float64(total)
		} else {
			health.SuccessRate = 100
		}
	}

	return result, nil
}

func normalizeCredentialsOptions(options CredentialsOptions) CredentialsOptions {
	if options.Now.IsZero() {
		options.Now = time.Now().UTC()
	} else {
		options.Now = options.Now.UTC()
	}
	if !options.Since.IsZero() {
		options.Since = options.Since.UTC()
	}
	if strings.TrimSpace(options.Range) == "" {
		options.Range = "all"
	}
	return options
}

func (o CredentialsOptions) healthWindowStart() time.Time {
	windowStart := o.Now.Add(-usageCredentialHealthWindowDuration)
	if !o.Since.IsZero() && o.Since.After(windowStart) {
		return o.Since
	}
	return windowStart
}

func newCredentialHealthSnapshot(windowStart, now time.Time) HealthSnapshot {
	rates := make([]int, usageCredentialHealthBucketCount)
	for index := range rates {
		rates[index] = -1
	}
	return HealthSnapshot{
		Rates:         rates,
		SuccessCounts: make([]int64, usageCredentialHealthBucketCount),
		FailureCounts: make([]int64, usageCredentialHealthBucketCount),
		WindowStart:   windowStart,
		WindowEnd:     now,
		BucketMinutes: usageCredentialHealthBucketMinutes,
		Rows:          usageCredentialHealthRows,
		Cols:          usageCredentialHealthCols,
		SuccessRate:   100,
	}
}

func queryCredentialAggregateRows(
	ctx context.Context,
	db *sql.DB,
	options CredentialsOptions,
) ([]credentialAggregateRow, error) {
	filter := buildUsageRecordWindowFilter(options.Since, options.Now)
	query := `
		SELECT source,
		       auth_index,
		       COALESCE(SUM(CASE WHEN failed = 0 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN failed != 0 THEN 1 ELSE 0 END), 0)
		FROM usage_records
	`
	query, args := applyCredentialFilter(query, filter)
	query += "\nGROUP BY source, auth_index ORDER BY source ASC, auth_index ASC"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("usage statistics: query credential usage: %w", err)
	}
	defer rows.Close()

	result := make([]credentialAggregateRow, 0, 16)
	for rows.Next() {
		var row credentialAggregateRow
		if err := rows.Scan(&row.Source, &row.AuthIndex, &row.Success, &row.Failure); err != nil {
			return nil, fmt.Errorf("usage statistics: scan credential usage: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usage statistics: iterate credential usage: %w", err)
	}
	return result, nil
}

func queryCredentialHealthBuckets(
	ctx context.Context,
	db *sql.DB,
	options CredentialsOptions,
) ([]credentialHealthBucketRow, error) {
	windowStart := options.healthWindowStart()
	bucketSeconds := int64(time.Duration(usageCredentialHealthBucketMinutes) * time.Minute / time.Second)
	filter := buildUsageRecordWindowFilter(windowStart, options.Now)
	query := `
		SELECT source,
		       auth_index,
		       (? - CAST(((julianday(?) - julianday(timestamp_utc)) * 86400.0) / ? AS INTEGER)) AS bucket_index,
		       COALESCE(SUM(CASE WHEN failed = 0 THEN 1 ELSE 0 END), 0) AS success_count,
		       COALESCE(SUM(CASE WHEN failed != 0 THEN 1 ELSE 0 END), 0) AS failure_count
		FROM usage_records
	`
	query, args := applyCredentialFilter(query, filter)
	query += "\nGROUP BY source, auth_index, bucket_index ORDER BY source ASC, auth_index ASC, bucket_index ASC"

	queryArgs := append(
		[]any{
			usageCredentialHealthBucketCount - 1,
			options.Now.Format(time.RFC3339Nano),
			bucketSeconds,
		},
		args...,
	)
	rows, err := db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("usage statistics: query credential health buckets: %w", err)
	}
	defer rows.Close()

	result := make([]credentialHealthBucketRow, 0, 32)
	for rows.Next() {
		var row credentialHealthBucketRow
		if err := rows.Scan(
			&row.Source,
			&row.AuthIndex,
			&row.Index,
			&row.SuccessCount,
			&row.FailureCount,
		); err != nil {
			return nil, fmt.Errorf("usage statistics: scan credential health bucket: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usage statistics: iterate credential health buckets: %w", err)
	}
	return result, nil
}

func applyCredentialFilter(query string, filter usageRecordFilter) (string, []any) {
	clauses := []string{"(source != '' OR auth_index != '')"}
	args := make([]any, 0, len(filter.args))
	if filter.whereClause != "" {
		filterClause := strings.TrimSpace(strings.TrimPrefix(filter.whereClause, "WHERE"))
		if filterClause != "" {
			clauses = append(clauses, filterClause)
			args = append(args, filter.args...)
		}
	}
	return query + "\nWHERE " + strings.Join(clauses, " AND "), args
}

func credentialUsageKey(source, authIndex string) string {
	return source + "\x00" + authIndex
}
