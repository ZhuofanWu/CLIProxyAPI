package usage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"
)

const (
	usageHealthRows          = 7
	usageHealthCols          = 96
	usageHealthBucketMinutes = 15
	usageHealthBucketCount   = usageHealthRows * usageHealthCols
)

var usageHealthWindowDuration = time.Duration(usageHealthBucketCount*usageHealthBucketMinutes) * time.Minute

func (s *sqliteStore) HealthContext(ctx context.Context, options HealthOptions) (HealthSnapshot, error) {
	db, err := s.openDatabase()
	if err != nil {
		if errors.Is(err, sql.ErrConnDone) {
			return HealthSnapshot{}, nil
		}
		return HealthSnapshot{}, err
	}
	defer db.Close()

	options = normalizeHealthOptions(options)
	result := newHealthSnapshot(options.Now)

	buckets, err := queryHealthBuckets(ctx, db, options)
	if err != nil {
		return HealthSnapshot{}, err
	}

	for _, bucket := range buckets {
		if bucket.Index < 0 || bucket.Index >= usageHealthBucketCount {
			continue
		}
		result.SuccessCounts[bucket.Index] = bucket.SuccessCount
		result.FailureCounts[bucket.Index] = bucket.FailureCount
		total := bucket.SuccessCount + bucket.FailureCount
		if total > 0 {
			result.Rates[bucket.Index] = int(math.Round(float64(bucket.SuccessCount) * 100 / float64(total)))
		}
		result.TotalSuccess += bucket.SuccessCount
		result.TotalFailure += bucket.FailureCount
	}

	total := result.TotalSuccess + result.TotalFailure
	if total > 0 {
		result.SuccessRate = float64(result.TotalSuccess) * 100 / float64(total)
	} else {
		result.SuccessRate = 100
	}

	return result, nil
}

func normalizeHealthOptions(options HealthOptions) HealthOptions {
	if options.Now.IsZero() {
		options.Now = time.Now().UTC()
	} else {
		options.Now = options.Now.UTC()
	}
	return options
}

func newHealthSnapshot(now time.Time) HealthSnapshot {
	rates := make([]int, usageHealthBucketCount)
	for index := range rates {
		rates[index] = -1
	}
	return HealthSnapshot{
		Rates:         rates,
		SuccessCounts: make([]int64, usageHealthBucketCount),
		FailureCounts: make([]int64, usageHealthBucketCount),
		WindowStart:   now.Add(-usageHealthWindowDuration),
		WindowEnd:     now,
		BucketMinutes: usageHealthBucketMinutes,
		Rows:          usageHealthRows,
		Cols:          usageHealthCols,
		SuccessRate:   100,
	}
}

type healthBucketRow struct {
	Index        int
	SuccessCount int64
	FailureCount int64
}

func queryHealthBuckets(ctx context.Context, db *sql.DB, options HealthOptions) ([]healthBucketRow, error) {
	windowStart := options.Now.Add(-usageHealthWindowDuration)
	bucketSeconds := int64(time.Duration(usageHealthBucketMinutes) * time.Minute / time.Second)
	query := `
		SELECT
			(? - CAST(((julianday(?) - julianday(timestamp_utc)) * 86400.0) / ? AS INTEGER)) AS bucket_index,
			COALESCE(SUM(CASE WHEN failed = 0 THEN 1 ELSE 0 END), 0) AS success_count,
			COALESCE(SUM(CASE WHEN failed != 0 THEN 1 ELSE 0 END), 0) AS failure_count
		FROM usage_records
		WHERE julianday(timestamp_utc) > julianday(?)
		  AND julianday(timestamp_utc) <= julianday(?)
		GROUP BY bucket_index
		ORDER BY bucket_index ASC
	`
	rows, err := db.QueryContext(
		ctx,
		query,
		usageHealthBucketCount-1,
		options.Now.Format(time.RFC3339Nano),
		bucketSeconds,
		windowStart.Format(time.RFC3339Nano),
		options.Now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, fmt.Errorf("usage statistics: query usage health buckets: %w", err)
	}
	defer rows.Close()

	buckets := make([]healthBucketRow, 0, 64)
	for rows.Next() {
		var bucket healthBucketRow
		if err := rows.Scan(&bucket.Index, &bucket.SuccessCount, &bucket.FailureCount); err != nil {
			return nil, fmt.Errorf("usage statistics: scan usage health bucket: %w", err)
		}
		buckets = append(buckets, bucket)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usage statistics: iterate usage health buckets: %w", err)
	}
	return buckets, nil
}
