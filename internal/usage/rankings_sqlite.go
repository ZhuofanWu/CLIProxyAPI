package usage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"
)

type rankingsRow struct {
	APIName      string
	ModelName    string
	Requests     int64
	SuccessCount int64
	FailureCount int64
	TotalTokens  int64
	InputTokens  int64
	OutputTokens int64
	CachedTokens int64
}

func (s *sqliteStore) RankingsContext(
	ctx context.Context,
	options RankingsOptions,
) (RankingsSnapshot, error) {
	db, err := s.openDatabase()
	if err != nil {
		if errors.Is(err, sql.ErrConnDone) {
			return RankingsSnapshot{}, nil
		}
		return RankingsSnapshot{}, err
	}
	defer db.Close()

	options = normalizeRankingsOptions(options)
	rows, err := queryRankingsRows(ctx, db, options)
	if err != nil {
		return RankingsSnapshot{}, err
	}

	apiRankings := make(map[string]*APIRanking, len(rows))
	modelRankings := make(map[string]*ModelRanking, len(rows))

	for _, row := range rows {
		rowCost := calculateGeneralCost(
			row.ModelName,
			row.InputTokens,
			row.OutputTokens,
			row.CachedTokens,
			options.ModelPrices,
		)

		apiItem := apiRankings[row.APIName]
		if apiItem == nil {
			apiItem = &APIRanking{
				APIName: row.APIName,
				Models:  make([]APIRankingModel, 0, 4),
			}
			apiRankings[row.APIName] = apiItem
		}
		apiItem.TotalRequests += row.Requests
		apiItem.SuccessCount += row.SuccessCount
		apiItem.FailureCount += row.FailureCount
		apiItem.TotalTokens += row.TotalTokens
		apiItem.TotalCost += rowCost
		apiItem.Models = append(apiItem.Models, APIRankingModel{
			ModelName:    row.ModelName,
			Requests:     row.Requests,
			SuccessCount: row.SuccessCount,
			FailureCount: row.FailureCount,
			Tokens:       row.TotalTokens,
		})

		modelItem := modelRankings[row.ModelName]
		if modelItem == nil {
			modelItem = &ModelRanking{ModelName: row.ModelName}
			modelRankings[row.ModelName] = modelItem
		}
		modelItem.Requests += row.Requests
		modelItem.SuccessCount += row.SuccessCount
		modelItem.FailureCount += row.FailureCount
		modelItem.Tokens += row.TotalTokens
		modelItem.Cost += rowCost
	}

	result := RankingsSnapshot{
		APIRankings:   make([]APIRanking, 0, len(apiRankings)),
		ModelRankings: make([]ModelRanking, 0, len(modelRankings)),
	}
	for _, item := range apiRankings {
		sort.SliceStable(item.Models, func(i, j int) bool {
			left := item.Models[i]
			right := item.Models[j]
			if left.Requests != right.Requests {
				return left.Requests > right.Requests
			}
			if left.Tokens != right.Tokens {
				return left.Tokens > right.Tokens
			}
			return left.ModelName < right.ModelName
		})
		result.APIRankings = append(result.APIRankings, *item)
	}
	for _, item := range modelRankings {
		result.ModelRankings = append(result.ModelRankings, *item)
	}

	sort.SliceStable(result.APIRankings, func(i, j int) bool {
		left := result.APIRankings[i]
		right := result.APIRankings[j]
		if left.TotalRequests != right.TotalRequests {
			return left.TotalRequests > right.TotalRequests
		}
		if left.TotalTokens != right.TotalTokens {
			return left.TotalTokens > right.TotalTokens
		}
		return left.APIName < right.APIName
	})
	sort.SliceStable(result.ModelRankings, func(i, j int) bool {
		left := result.ModelRankings[i]
		right := result.ModelRankings[j]
		if left.Requests != right.Requests {
			return left.Requests > right.Requests
		}
		if left.Tokens != right.Tokens {
			return left.Tokens > right.Tokens
		}
		return left.ModelName < right.ModelName
	})

	return result, nil
}

func normalizeRankingsOptions(options RankingsOptions) RankingsOptions {
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

func queryRankingsRows(ctx context.Context, db *sql.DB, options RankingsOptions) ([]rankingsRow, error) {
	filter := buildUsageRecordWindowFilter(options.Since, options.Now)
	query := `
		SELECT api_name,
		       model_name,
		       COUNT(*),
		       COALESCE(SUM(CASE WHEN failed = 0 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN failed != 0 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(total_tokens), 0),
		       COALESCE(SUM(input_tokens), 0),
		       COALESCE(SUM(output_tokens), 0),
		       COALESCE(SUM(cached_tokens), 0)
		FROM usage_records
	`
	if filter.whereClause != "" {
		query += "\n" + filter.whereClause + "\n"
	}
	query += "GROUP BY api_name, model_name ORDER BY api_name ASC, model_name ASC"

	rows, err := db.QueryContext(ctx, query, filter.args...)
	if err != nil {
		return nil, fmt.Errorf("usage statistics: query usage rankings: %w", err)
	}
	defer rows.Close()

	result := make([]rankingsRow, 0, 16)
	for rows.Next() {
		var row rankingsRow
		if err := rows.Scan(
			&row.APIName,
			&row.ModelName,
			&row.Requests,
			&row.SuccessCount,
			&row.FailureCount,
			&row.TotalTokens,
			&row.InputTokens,
			&row.OutputTokens,
			&row.CachedTokens,
		); err != nil {
			return nil, fmt.Errorf("usage statistics: scan usage rankings: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usage statistics: iterate usage rankings: %w", err)
	}
	return result, nil
}
