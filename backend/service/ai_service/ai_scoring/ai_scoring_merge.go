package ai_scoring

import "strings"

func normalizeState(state *scoringState, fallbackArticleType string, chunkIndex int, chunkCount int) *scoringState {
	if state == nil {
		state = &scoringState{}
	}
	state.ArticleType = normalizeArticleTypeWithFallback(state.ArticleType, fallbackArticleType)
	state.ProvisionalDimensions = normalizeStateDimensions(state.ProvisionalDimensions)
	state.MainIssues = normalizeIssuesPreserveOrder(state.MainIssues)
	state.ChunkSummary = strings.TrimSpace(state.ChunkSummary)
	state.GlobalSummary = strings.TrimSpace(state.GlobalSummary)
	state.OverallComment = strings.TrimSpace(state.OverallComment)
	state.CoveredChunkIndex = chunkIndex
	state.CoveredChunkCount = chunkCount
	return state
}

func normalizeFinalResponse(resp *ArticleScoreResponse, fallbackArticleType string) *ArticleScoreResponse {
	if resp == nil {
		resp = &ArticleScoreResponse{}
	}

	resp.ArticleType = normalizeArticleTypeWithFallback(resp.ArticleType, fallbackArticleType)
	resp.Dimensions = normalizeFinalDimensions(resp.Dimensions)
	resp.MainIssues = normalizeIssuesPreserveOrder(resp.MainIssues)
	resp.OverallComment = strings.TrimSpace(resp.OverallComment)

	if resp.AITotalScore == 0 && resp.TotalScore != 0 {
		resp.AITotalScore = resp.TotalScore
	}
	resp.AITotalScore = clampScore(resp.AITotalScore)
	resp.TotalScore = calculateWeightedScore(resp.Dimensions, resp.ArticleType)
	resp.ScoreLevel = scoreLevel(resp.TotalScore)
	resp.MainIssues = limitIssuesByScore(resp.MainIssues, resp.TotalScore)
	return resp
}

func normalizeStateDimensions(list []scoringStateDimension) []scoringStateDimension {
	itemMap := make(map[string]scoringStateDimension, len(list))
	for _, item := range list {
		name := strings.TrimSpace(strings.ToLower(item.Name))
		if name == "" {
			continue
		}
		item.Name = name
		item.Score = clampScore(item.Score)
		item.Evidence = strings.TrimSpace(item.Evidence)
		itemMap[name] = item
	}

	result := make([]scoringStateDimension, 0, len(dimensionOrder))
	for _, name := range dimensionOrder {
		item, ok := itemMap[name]
		if !ok {
			item = scoringStateDimension{
				Name:     name,
				Score:    50,
				Evidence: "未提供该维度证据，按保守中间分处理",
			}
		}
		if item.Evidence == "" {
			item.Evidence = "未提供该维度证据"
		}
		result = append(result, item)
	}
	return result
}

func normalizeFinalDimensions(list []ArticleScoreDimension) []ArticleScoreDimension {
	itemMap := make(map[string]ArticleScoreDimension, len(list))
	for _, item := range list {
		name := strings.TrimSpace(strings.ToLower(item.Name))
		if name == "" {
			continue
		}
		item.Name = name
		item.Score = clampScore(item.Score)
		item.Reason = strings.TrimSpace(item.Reason)
		itemMap[name] = item
	}

	result := make([]ArticleScoreDimension, 0, len(dimensionOrder))
	for _, name := range dimensionOrder {
		item, ok := itemMap[name]
		if !ok {
			item = ArticleScoreDimension{
				Name:   name,
				Score:  50,
				Reason: "未提供该维度理由，按保守中间分处理",
			}
		}
		if item.Reason == "" {
			item.Reason = "未提供该维度理由"
		}
		result = append(result, item)
	}
	return result
}

func normalizeIssuesPreserveOrder(list []ArticleScoreIssue) []ArticleScoreIssue {
	if len(list) == 0 {
		return nil
	}

	result := make([]ArticleScoreIssue, 0, len(list))
	for _, item := range list {
		item.Reason = strings.TrimSpace(item.Reason)
		item.Suggestion = strings.TrimSpace(item.Suggestion)
		if item.Reason == "" && item.Suggestion == "" {
			continue
		}
		item.Positions = normalizePositionsPreserveOrder(item.Positions)
		result = append(result, item)
	}
	return result
}

func normalizePositionsPreserveOrder(list []ArticleScorePosition) []ArticleScorePosition {
	if len(list) == 0 {
		return nil
	}

	result := make([]ArticleScorePosition, 0, len(list))
	for _, item := range list {
		item.Paragraph = maxInt(item.Paragraph, 0)
		item.Quote = strings.TrimSpace(item.Quote)
		result = append(result, item)
	}
	return result
}

func issueCountRange(score int) (min int, max int) {
	switch {
	case score < 30:
		return 5, 9
	case score < 60:
		return 4, 7
	case score < 70:
		return 2, 5
	default:
		return 0, 3
	}
}

func limitIssuesByScore(list []ArticleScoreIssue, score int) []ArticleScoreIssue {
	if len(list) == 0 {
		return nil
	}
	_, maxIssues := issueCountRange(score)
	maxIssues = minInt(maxIssues, articleScoringMaxIssues)
	if maxIssues > 0 && len(list) > maxIssues {
		return list[:maxIssues]
	}
	return list
}

func calculateWeightedScore(dimensions []ArticleScoreDimension, articleType string) int {
	weights := scoringWeights(articleType)
	scoreMap := make(map[string]int, len(dimensions))
	for _, item := range dimensions {
		scoreMap[strings.ToLower(item.Name)] = clampScore(item.Score)
	}

	totalWeight := 0
	totalScore := 0
	for _, name := range dimensionOrder {
		weight := weights[name]
		totalWeight += weight
		totalScore += scoreMap[name] * weight
	}
	if totalWeight == 0 {
		return 50
	}
	return clampScore(totalScore / totalWeight)
}

func clampScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func scoreLevel(score int) string {
	switch {
	case score <= 30:
		return "较差文章"
	case score <= 59:
		return "不合格文章"
	case score <= 80:
		return "常规文章"
	case score <= 89:
		return "优质文章"
	default:
		return "精品文章"
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
