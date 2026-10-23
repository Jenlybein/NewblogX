package ai_scoring

import (
	"fmt"
	"myblogx/utils/markdown"
	"regexp"
	"strings"
)

var fencedCodeRegexp = regexp.MustCompile("(?s)```.*?```")

// prepareArticleForScoring 清洗文章内容，尽量保留正文与标题信息，避免 Markdown 噪音影响评分。
func prepareArticleForScoring(title string, content string) (string, string, []string) {
	headings := buildHeadingList(content)
	title = strings.TrimSpace(title)
	if title == "" && len(headings) > 0 {
		title = headings[0]
	}

	cleaned := fencedCodeRegexp.ReplaceAllString(content, "\n代码示例已省略\n")
	cleaned = markdown.MdToPlainText(cleaned)
	cleaned = strings.ReplaceAll(cleaned, "\r\n", "\n")
	cleaned = strings.ReplaceAll(cleaned, "\r", "\n")
	cleaned = strings.TrimSpace(cleaned)

	if title == "" {
		title = markdown.ExtractText(markdown.MdToTextParagraph(cleaned), 30)
	}
	if title == "" {
		title = "未命名文章"
	}

	return title, cleaned, headings
}

func buildHeadingList(content string) []string {
	partList := markdown.MdToContentParts(content)
	if len(partList) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(partList))
	list := make([]string, 0, len(partList))
	for _, part := range partList {
		if part.Level <= 0 || len(part.Path) == 0 {
			continue
		}
		item := strings.TrimSpace(strings.Join(part.Path, " > "))
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		list = append(list, item)
	}
	return list
}

func buildArticleParagraphs(content string) []articleParagraph {
	lines := strings.Split(content, "\n")
	result := make([]articleParagraph, 0, len(lines))
	index := 1
	for _, line := range lines {
		line = strings.Join(strings.Fields(strings.TrimSpace(line)), " ")
		if line == "" {
			continue
		}
		result = append(result, articleParagraph{
			Number: index,
			Text:   line,
		})
		index++
	}
	return result
}

func splitParagraphFragments(paragraph articleParagraph, maxChars int) []string {
	prefix := fmt.Sprintf("[P%d] ", paragraph.Number)
	body := strings.TrimSpace(paragraph.Text)
	if body == "" {
		return nil
	}

	bodyRunes := []rune(body)
	limit := maxChars - len([]rune(prefix))
	if limit <= 32 {
		limit = maxChars
	}
	if len(bodyRunes) <= limit {
		return []string{prefix + body}
	}

	result := make([]string, 0, len(bodyRunes)/limit+1)
	for start := 0; start < len(bodyRunes); start += limit {
		end := start + limit
		if end > len(bodyRunes) {
			end = len(bodyRunes)
		}
		result = append(result, prefix+string(bodyRunes[start:end]))
	}
	return result
}

func splitArticleChunks(content string) []scoringChunk {
	paragraphs := buildArticleParagraphs(content)
	if len(paragraphs) == 0 {
		return nil
	}

	fragments := make([]string, 0, len(paragraphs))
	totalRunes := 0
	for _, paragraph := range paragraphs {
		partList := splitParagraphFragments(paragraph, articleScoringChunkMaxChars)
		fragments = append(fragments, partList...)
		for _, item := range partList {
			totalRunes += len([]rune(item))
		}
	}
	if len(fragments) == 0 {
		return nil
	}

	chunkCount := (totalRunes + articleScoringChunkMaxChars - 1) / articleScoringChunkMaxChars
	if chunkCount <= 0 {
		chunkCount = 1
	}
	targetChars := (totalRunes + chunkCount - 1) / chunkCount
	if targetChars > articleScoringChunkMaxChars {
		targetChars = articleScoringChunkMaxChars
	}

	chunkList := make([]scoringChunk, 0, chunkCount)
	current := make([]string, 0, 8)
	currentRunes := 0
	appendChunk := func() {
		if len(current) == 0 {
			return
		}
		chunkList = append(chunkList, scoringChunk{
			Index:   len(chunkList) + 1,
			Content: strings.Join(current, "\n\n"),
		})
		current = current[:0]
		currentRunes = 0
	}

	for _, fragment := range fragments {
		length := len([]rune(fragment))
		if currentRunes == 0 {
			current = append(current, fragment)
			currentRunes = length
			continue
		}

		nextRunes := currentRunes + 2 + length
		if nextRunes > articleScoringChunkMaxChars {
			appendChunk()
			current = append(current, fragment)
			currentRunes = length
			continue
		}

		beforeDiff := absInt(targetChars - currentRunes)
		afterDiff := absInt(targetChars - nextRunes)
		if currentRunes >= targetChars && beforeDiff <= afterDiff {
			appendChunk()
			current = append(current, fragment)
			currentRunes = length
			continue
		}

		current = append(current, fragment)
		currentRunes = nextRunes
	}

	appendChunk()
	for index := range chunkList {
		chunkList[index].Index = index + 1
	}
	return chunkList
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
