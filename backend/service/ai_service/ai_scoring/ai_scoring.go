package ai_scoring

import (
	"fmt"
	"myblogx/conf"
	"myblogx/service/ai_service"
	"strings"
)

// ScoreArticleQuality 对整篇文章做质量评分与写作建议分析。
func ScoreArticleQuality(aiConf conf.AI, req ArticleScoreRequest) (*ArticleScoreResponse, error) {
	title, content, headings := prepareArticleForScoring(req.Title, req.Content)
	if len([]rune(strings.TrimSpace(content))) < articleScoringMinChars {
		return nil, fmt.Errorf("文章内容过短，建议补充完整后再评分")
	}

	if len([]rune(content)) <= articleScoringDirectMaxChars {
		return scoreShortArticle(aiConf, title, content, headings)
	}
	return scoreLongArticle(aiConf, title, content, headings)
}

func scoreShortArticle(aiConf conf.AI, title string, content string, headings []string) (*ArticleScoreResponse, error) {
	reply, err := ai_service.Chat(aiConf, []ai_service.Message{
		{
			Role:    "system",
			Content: buildFullArticlePrompt(title, content, headings),
		},
	})
	if err != nil {
		return nil, err
	}

	var response ArticleScoreResponse
	if err = ai_service.UnmarshalJSONBlock(reply, &response); err != nil {
		return nil, fmt.Errorf("全文评分结果不是有效 JSON: %w", err)
	}
	return normalizeFinalResponse(&response, ""), nil
}

func scoreLongArticle(aiConf conf.AI, title string, content string, headings []string) (*ArticleScoreResponse, error) {
	chunkList := splitArticleChunks(content)
	if len(chunkList) == 0 {
		return nil, fmt.Errorf("文章内容过短，建议补充完整后再评分")
	}

	totalChunks := len(chunkList)
	reply, err := ai_service.Chat(aiConf, []ai_service.Message{
		{
			Role:    "system",
			Content: buildFirstChunkPrompt(title, chunkList[0], totalChunks),
		},
	})
	if err != nil {
		return nil, err
	}

	var state scoringState
	if err = ai_service.UnmarshalJSONBlock(reply, &state); err != nil {
		return nil, fmt.Errorf("长文首段评分结果不是有效 JSON: %w", err)
	}
	statePtr := normalizeState(&state, "", 1, totalChunks)

	for index := 1; index < totalChunks-1; index++ {
		reply, err = ai_service.Chat(aiConf, []ai_service.Message{
			{
				Role:    "system",
				Content: buildMiddleChunkPrompt(title, chunkList[index], totalChunks, statePtr),
			},
		})
		if err != nil {
			return nil, err
		}

		var nextState scoringState
		if err = ai_service.UnmarshalJSONBlock(reply, &nextState); err != nil {
			return nil, fmt.Errorf("长文中间段评分结果不是有效 JSON: %w", err)
		}
		statePtr = normalizeState(&nextState, statePtr.ArticleType, index+1, totalChunks)
	}

	finalChunk := chunkList[totalChunks-1]
	reply, err = ai_service.Chat(aiConf, []ai_service.Message{
		{
			Role:    "system",
			Content: buildFinalChunkPrompt(title, finalChunk, totalChunks, statePtr, headings),
		},
	})
	if err != nil {
		return nil, err
	}

	var response ArticleScoreResponse
	if err = ai_service.UnmarshalJSONBlock(reply, &response); err != nil {
		return nil, fmt.Errorf("长文最终评分结果不是有效 JSON: %w", err)
	}
	return normalizeFinalResponse(&response, statePtr.ArticleType), nil
}
