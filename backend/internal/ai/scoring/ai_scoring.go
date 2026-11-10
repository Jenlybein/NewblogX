package scoring

import (
	"context"
	"fmt"
	"myblogx/conf"
	"myblogx/internal/ai"
	"strings"
)

// ScoreArticleQuality 对整篇文章做质量评分与写作建议分析。
//
// **温度固定为 0**（票 306 / ADR-0032）：评分是要拿去做"能不能发布"的判定，
// 需要的是可复现而不是创造性——同一篇文章、同一份提示词连调两次给出 47 与
// 53 时，作者看到的是"随机打回"。站点级 ai.temperature 是给对话类场景用的，
// 评分场景不跟随它；这一处覆盖只影响评分，捏捏、改写、诊断等场景仍然吃配置值。
func ScoreArticleQuality(ctx context.Context, aiConf conf.AI, req ArticleScoreRequest) (*ArticleScoreResponse, error) {
	aiConf.Temperature = 0

	title, content, headings := prepareArticleForScoring(req.Title, req.Content)
	if len([]rune(strings.TrimSpace(content))) < articleScoringMinChars {
		return nil, fmt.Errorf("文章内容过短，建议补充完整后再评分")
	}

	// 形态事实只算一次，封顶在两个出口共用（ADR-0040）。
	shape := analyzeContentShape(content)

	if len([]rune(content)) <= articleScoringDirectMaxChars {
		return scoreShortArticle(ctx, aiConf, req.Policy, title, content, headings, shape)
	}
	return scoreLongArticle(ctx, aiConf, req.Policy, title, content, headings, shape)
}

func scoreShortArticle(
	ctx context.Context, aiConf conf.AI, policy string,
	title string, content string, headings []string, shape contentShape,
) (*ArticleScoreResponse, error) {
	reply, err := ai.Chat(ctx, aiConf, []ai.Message{
		{
			Role:    "system",
			Content: buildFullArticlePrompt(policy, title, content, headings),
		},
	})
	if err != nil {
		return nil, err
	}

	var response ArticleScoreResponse
	if err = ai.UnmarshalJSONBlock(reply, &response); err != nil {
		return nil, fmt.Errorf("全文评分结果不是有效 JSON: %w", err)
	}
	return finalizeResponse(&response, "", shape), nil
}

func scoreLongArticle(
	ctx context.Context, aiConf conf.AI, policy string,
	title string, content string, headings []string, shape contentShape,
) (*ArticleScoreResponse, error) {
	chunkList := splitArticleChunks(content)
	if len(chunkList) == 0 {
		return nil, fmt.Errorf("文章内容过短，建议补充完整后再评分")
	}

	totalChunks := len(chunkList)
	reply, err := ai.Chat(ctx, aiConf, []ai.Message{
		{
			Role:    "system",
			Content: buildFirstChunkPrompt(policy, title, chunkList[0], totalChunks),
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
				Content: buildMiddleChunkPrompt(policy, title, chunkList[index], totalChunks, statePtr),
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
			Content: buildFinalChunkPrompt(policy, title, finalChunk, totalChunks, statePtr, headings),
		},
	})
	if err != nil {
		return nil, err
	}

	var response ArticleScoreResponse
	if err = ai.UnmarshalJSONBlock(reply, &response); err != nil {
		return nil, fmt.Errorf("长文最终评分结果不是有效 JSON: %w", err)
	}
	return finalizeResponse(&response, statePtr.ArticleType, shape), nil
}

// finalizeResponse 是评分结果的唯一收口：先按契约归一化，再套形态封顶。
//
// 两者分开写在两个文件里是有意的——归一化保证"结果读得出来"，封顶保证
// "读出来的分不会被常识否掉"（ADR-0040）。
func finalizeResponse(resp *ArticleScoreResponse, fallbackArticleType string, shape contentShape) *ArticleScoreResponse {
	out := normalizeFinalResponse(resp, fallbackArticleType)
	applyShapeCaps(out, shape)
	return out
}
