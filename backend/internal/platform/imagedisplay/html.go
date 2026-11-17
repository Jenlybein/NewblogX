package imagedisplay

import "strings"

// blockedImagePlaceholder 是"这张图不该被看到"时替换成的节点。
//
// 为什么给一整段 HTML 而不是留空：留空会让正文排版塌掉（相邻段落合并、
// 图片说明文字悬空）。给一个固定尺寸的占位块，读者看到的是"这里本来有图"，
// 而不是"这段文字为什么挤在一起"。
//
// 文案刻意不带内部状态（不写"审核中"还是"已屏蔽"）：读者不需要知道运维细节，
// 区分状态只会让人来追问"为什么我这篇被审了"。
const blockedImagePlaceholder = `<span class="image-unavailable" role="img" aria-label="图片不可用"></span>`

// ExtractImgSrcs 从 HTML 里按出现顺序取出所有 `<img>` 的 src。
//
// 只做"找 src"这一件事，不解析 DOM：正文是受控的（markdown 渲染后经白名单
// 清洗），<img> 的属性顺序可能变化，所以这里容忍 src 出现在任意位置，
// 也容忍单双引号。
func ExtractImgSrcs(html string) []string {
	tags := imgTagSpans(html)
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if src := srcOf(tag.tag); src != "" {
			out = append(out, src)
		}
	}
	return out
}

// RewriteImgSrcs 按映射重写 HTML 里的 `<img>`。
//
// 映射的取值有三种语义：
//   - 不在映射里：这个地址不归我们管（外链），原样保留；
//   - 空串：这张图不该被看到，整段 <img> 换成占位块；
//   - 非空：用新地址替换 src，其余属性（alt / 宽高 / class）保持不变。
//
// 返回重写后的 HTML。
func RewriteImgSrcs(html string, resolved map[string]string) string {
	tags := imgTagSpans(html)
	if len(tags) == 0 {
		return html
	}

	var builder strings.Builder
	builder.Grow(len(html))
	cursor := 0
	for _, tag := range tags {
		src := srcOf(tag.tag)
		target, ok := resolved[src]
		if src == "" || !ok {
			// 找不到 src 或不是我们的图：整段原样保留。
			continue
		}
		builder.WriteString(html[cursor:tag.start])
		if target == "" {
			builder.WriteString(blockedImagePlaceholder)
		} else {
			builder.WriteString(replaceSrc(tag.tag, target))
		}
		cursor = tag.end
	}
	if cursor == 0 {
		return html
	}
	builder.WriteString(html[cursor:])
	return builder.String()
}

// imgSpan 是一段 `<img ...>` 在原文里的位置。
type imgSpan struct {
	start int
	end   int
	tag   string
}

// imgTagSpans 找出所有 <img ...> 标签的位置（大小写不敏感）。
func imgTagSpans(html string) []imgSpan {
	lower := strings.ToLower(html)
	var out []imgSpan
	from := 0
	for {
		idx := strings.Index(lower[from:], "<img")
		if idx < 0 {
			return out
		}
		start := from + idx
		// 必须紧跟空白或 '>'，避免把 <image-x> 之类的自定义标签算进来。
		after := start + len("<img")
		if after < len(lower) {
			next := lower[after]
			if next != ' ' && next != '\t' && next != '\n' && next != '\r' && next != '>' && next != '/' {
				from = after
				continue
			}
		}
		end := strings.Index(lower[start:], ">")
		if end < 0 {
			return out
		}
		end = start + end + 1
		out = append(out, imgSpan{start: start, end: end, tag: html[start:end]})
		from = end
	}
}

// srcOf 从一段 <img ...> 里取出 src 的值（单双引号都认）。
func srcOf(tag string) string {
	lower := strings.ToLower(tag)
	idx := strings.Index(lower, "src=")
	if idx < 0 {
		return ""
	}
	rest := tag[idx+len("src="):]
	rest = strings.TrimLeft(rest, " \t\r\n")
	if rest == "" {
		return ""
	}
	quote := rest[0]
	if quote == '"' || quote == '\'' {
		rest = rest[1:]
		if end := strings.IndexByte(rest, quote); end >= 0 {
			return strings.TrimSpace(rest[:end])
		}
		return ""
	}
	// 无引号：读到空白或 '>' 为止。
	end := strings.IndexAny(rest, " \t\r\n>")
	if end < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}

// replaceSrc 把标签里的 src 值换成新地址，保留引号风格与其余属性。
func replaceSrc(tag, newSrc string) string {
	lower := strings.ToLower(tag)
	idx := strings.Index(lower, "src=")
	if idx < 0 {
		return tag
	}
	head := tag[:idx+len("src=")]
	rest := tag[idx+len("src="):]
	trimmed := strings.TrimLeft(rest, " \t\r\n")
	spaces := rest[:len(rest)-len(trimmed)]
	if trimmed == "" {
		return tag
	}
	quote := trimmed[0]
	if quote == '"' || quote == '\'' {
		body := trimmed[1:]
		if end := strings.IndexByte(body, quote); end >= 0 {
			return head + spaces + string(quote) + newSrc + string(quote) + body[end+1:]
		}
		return tag
	}
	end := strings.IndexAny(trimmed, " \t\r\n>")
	if end < 0 {
		return head + spaces + newSrc
	}
	return head + spaces + newSrc + trimmed[end:]
}
