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
	defaultUsageEventsPageSize = 100
	maxUsageEventsPageSize     = 500
)

func (s *sqliteStore) EventsContext(
	ctx context.Context,
	options UsageEventsOptions,
) (UsageEventsSnapshot, error) {
	db, err := s.openDatabase()
	if err != nil {
		if errors.Is(err, sql.ErrConnDone) {
			return UsageEventsSnapshot{}, nil
		}
		return UsageEventsSnapshot{}, err
	}
	defer db.Close()

	options = normalizeUsageEventsOptions(options)
	total, err := queryUsageEventsCount(ctx, db, options)
	if err != nil {
		return UsageEventsSnapshot{}, err
	}

	totalPages := 1
	if total > 0 {
		totalPages = int((total + int64(options.PageSize) - 1) / int64(options.PageSize))
	}
	if options.Page > totalPages {
		options.Page = totalPages
	}

	items, err := queryUsageEventItems(ctx, db, options)
	if err != nil {
		return UsageEventsSnapshot{}, err
	}

	return UsageEventsSnapshot{
		Page:       options.Page,
		PageSize:   options.PageSize,
		Total:      total,
		TotalPages: totalPages,
		HasPrev:    options.Page > 1,
		HasNext:    options.Page < totalPages,
		Items:      items,
	}, nil
}

func normalizeUsageEventsOptions(options UsageEventsOptions) UsageEventsOptions {
	if !options.Since.IsZero() {
		options.Since = options.Since.UTC()
	}
	if options.Now.IsZero() {
		options.Now = time.Now().UTC()
	} else {
		options.Now = options.Now.UTC()
	}
	options.ModelName = strings.TrimSpace(options.ModelName)
	options.Source = strings.TrimSpace(options.Source)
	options.AuthIndex = strings.TrimSpace(options.AuthIndex)
	if options.Page < 1 {
		options.Page = 1
	}
	switch {
	case options.PageSize <= 0:
		options.PageSize = defaultUsageEventsPageSize
	case options.PageSize > maxUsageEventsPageSize:
		options.PageSize = maxUsageEventsPageSize
	}
	return options
}

func buildUsageEventsFilter(options UsageEventsOptions) usageRecordFilter {
	windowFilter := buildUsageRecordWindowFilter(options.Since, options.Now)
	clauses := make([]string, 0, 5)
	args := make([]any, 0, len(windowFilter.args)+3)
	if windowFilter.whereClause != "" {
		windowClause := strings.TrimSpace(strings.TrimPrefix(windowFilter.whereClause, "WHERE"))
		if windowClause != "" {
			clauses = append(clauses, windowClause)
			args = append(args, windowFilter.args...)
		}
	}
	if options.ModelName != "" {
		clauses = append(clauses, "model_name = ?")
		args = append(args, options.ModelName)
	}
	if options.Source != "" {
		clauses = append(clauses, "source = ?")
		args = append(args, options.Source)
	}
	if options.AuthIndex != "" {
		clauses = append(clauses, "auth_index = ?")
		args = append(args, options.AuthIndex)
	}
	if options.Success != nil {
		if *options.Success {
			clauses = append(clauses, "failed = 0")
		} else {
			clauses = append(clauses, "failed != 0")
		}
	}
	if len(clauses) == 0 {
		return usageRecordFilter{}
	}
	return usageRecordFilter{
		whereClause: "WHERE " + strings.Join(clauses, " AND "),
		args:        args,
	}
}

func queryUsageEventsCount(
	ctx context.Context,
	db *sql.DB,
	options UsageEventsOptions,
) (int64, error) {
	filter := buildUsageEventsFilter(options)
	query := `SELECT COUNT(*) FROM usage_records`
	if filter.whereClause != "" {
		query += "\n" + filter.whereClause
	}

	var total int64
	if err := db.QueryRowContext(ctx, query, filter.args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("usage statistics: query usage events count: %w", err)
	}
	return total, nil
}

func queryUsageEventItems(
	ctx context.Context,
	db *sql.DB,
	options UsageEventsOptions,
) ([]UsageEventItem, error) {
	filter := buildUsageEventsFilter(options)
	args := append([]any{}, filter.args...)
	args = append(args, options.PageSize, (options.Page-1)*options.PageSize)

	query := `
		SELECT api_name,
		       model_name,
		       timestamp_utc,
		       source,
		       auth_index,
		       failed,
		       input_tokens,
		       output_tokens,
		       reasoning_tokens,
		       cached_tokens,
		       total_tokens
		FROM usage_records
	`
	if filter.whereClause != "" {
		query += "\n" + filter.whereClause + "\n"
	}
	query += `
		ORDER BY julianday(timestamp_utc) DESC, api_name ASC, model_name ASC, source ASC, auth_index ASC
		LIMIT ? OFFSET ?
	`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("usage statistics: query usage events items: %w", err)
	}
	defer rows.Close()

	items := make([]UsageEventItem, 0, options.PageSize)
	for rows.Next() {
		var (
			item         UsageEventItem
			timestampUTC string
			failedInt    int64
		)
		if err := rows.Scan(
			&item.APIName,
			&item.ModelName,
			&timestampUTC,
			&item.Source,
			&item.AuthIndex,
			&failedInt,
			&item.Tokens.InputTokens,
			&item.Tokens.OutputTokens,
			&item.Tokens.ReasoningTokens,
			&item.Tokens.CachedTokens,
			&item.Tokens.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("usage statistics: scan usage event item: %w", err)
		}
		parsedTime, err := time.Parse(time.RFC3339Nano, timestampUTC)
		if err != nil {
			return nil, fmt.Errorf("usage statistics: parse usage event timestamp: %w", err)
		}
		item.Timestamp = parsedTime.UTC()
		item.Failed = failedInt != 0
		item.Tokens = normaliseTokenStats(item.Tokens)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usage statistics: iterate usage event items: %w", err)
	}
	return items, nil
}
