package scoring

import (
	"context"
	"fmt"
	"myblogx/conf"
	"myblogx/internal/ai"
	"strings"
)

// ScoreArticleQuality 对整篇文章做质量评分与写作建议分析。
func ScoreArticleQuality(ctx context.Context, aiConf conf.AI, req ArticleScoreRequest) (*ArticleScoreResponse, error) {
	title, content, headings := prepareArticleForScoring(req.Title, req.Content)
	if len([]rune(strings.TrimSpace(content))) < articleScoringMinChars {
		return nil, fmt.Errorf("文章内容过短，建议补充完整后再评分")
	}

	if len([]rune(content)) <= articleScoringDirectMaxChars {
		return scoreShortArticle(ctx, aiConf, title, content, headings)
	}
	return scoreLongArticle(ctx, aiConf, title, content, headings)
}

func scoreShortArticle(ctx context.Context, aiConf conf.AI, title string, content string, headings []string) (*ArticleScoreResponse, error) {
	reply, err := ai.Chat(ctx, aiConf, []ai.Message{
		{
			Role:    "system",
			Content: buildFullArticlePrompt(title, content, headings),
		},
	})
	if err != nil {
		return nil, err
	}

	var response ArticleScoreResponse
	if err = ai.UnmarshalJSONBlock(reply, &response); err != nil {
		return nil, fmt.Errorf("全文评分结果不是有效 JSON: %w", err)
	}
	return normalizeFinalResponse(&response, ""), nil
}

func scoreLongArticle(ctx context.Context, aiConf conf.AI, title string, content string, headings []string) (*ArticleScoreResponse, error) {
	chunkList := splitArticleChunks(content)
	if len(chunkList) == 0 {
		return nil, fmt.Errorf("文章内容过短，建议补充完整后再评分")
	}

	totalChunks := len(chunkList)
	reply, err := ai.Chat(ctx, aiConf, []ai.Message{
		{
			Role:    "system",
			Content: buildFirstChunkPrompt(title, chunkList[0], totalChunks),
		},
	})
	if err != nil {
		return nil, err
	}

	var state scoringState
	if err = ai.UnmarshalJSONBlock(reply, &state); err != nil {
		return nil, fmt.Errorf("长文首段评分结果不是有效 JSON: %w", err)
	}
	statePtr := normalizeState(&state, "", 1, totalChunks)

	for index := 1; index < totalChunks-1; index++ {
		reply, err = ai.Chat(ctx, aiConf, []ai.Message{
			{
				Role:    "system",
				Content: buildMiddleChunkPrompt(title, chunkList[index], totalChunks, statePtr),
			},
		})
		if err != nil {
			return nil, err
		}

		var nextState scoringState
		if err = ai.UnmarshalJSONBlock(reply, &nextState); err != nil {
			return nil, fmt.Errorf("长文中间段评分结果不是有效 JSON: %w", err)
		}
		statePtr = normalizeState(&nextState, statePtr.ArticleType, index+1, totalChunks)
	}

	finalChunk := chunkList[totalChunks-1]
	reply, err = ai.Chat(ctx, aiConf, []ai.Message{
		{
			Role:    "system",
			Content: buildFinalChunkPrompt(title, finalChunk, totalChunks, statePtr, headings),
		},
	})
	if err != nil {
		return nil, err
	}

	var response ArticleScoreResponse
	if err = ai.UnmarshalJSONBlock(reply, &response); err != nil {
		return nil, fmt.Errorf("长文最终评分结果不是有效 JSON: %w", err)
	}
	return normalizeFinalResponse(&response, statePtr.ArticleType), nil
}
