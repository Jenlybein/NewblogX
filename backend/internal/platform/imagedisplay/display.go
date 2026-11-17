// Package imagedisplay 是"图片展示地址"的唯一出口。
//
// 为什么需要它（ADR-0034）：站内图片地址在**写入时**就被快照进各业务表
// （文章 cover、用户 avatar、轮播 cover、合集/项目 cover、评论与私信里的头像……），
// 读面直接把这些字符串发给前端，没有任何二次判定。于是"压缩样式"和"审核闸门"
// 这两件都属于"读的时候才决定"的事，如果散在十几个映射函数里各写一遍，
// 迟早会出现某一处漏了闸门 —— 漏一处等于把未过审的图放出去。
//
// 所以这里只暴露一个批量入口：一批原地址 + 各自的用途槽位 + 观看者身份进来，
// 一批展示地址出去（空串 = 不要展示，由前端画占位）。
//
// 三条不变量：
//  1. 库里永远只存原图地址。样式是读取时派生的，所以清空样式名立刻回原图、
//     关闸门立刻全都可见，都不需要动数据。
//  2. 非本站存储的地址原样返回。外链图不能被加上七牛样式后缀（那是别人的 URL）。
//  3. 派生必须幂等：同一个地址反复派生不得长出 `-small-small`。
package imagedisplay

import (
	"context"
	"net/url"
	"strings"
)

// Purpose 是样式槽位。四档对应四个可独立配置的七牛样式名。
type Purpose string

const (
	// PurposeCover 文章封面 / 轮播封面 / 合集与项目封面 / 收藏夹封面。
	PurposeCover Purpose = "cover"
	// PurposeAvatar 用户头像（尺寸最小，单独一档）。
	PurposeAvatar Purpose = "avatar"
	// PurposeContent 文章正文内嵌图。
	PurposeContent Purpose = "content"
	// PurposeChat 私信图片。
	PurposeChat Purpose = "chat"
)

// Config 是派生所需的全部静态输入，来自运行时配置（每次现读，保存即生效）。
type Config struct {
	// MediaBase 是存储访问基址（已归一化成含协议头、无尾斜杠）。
	MediaBase string
	// Prefix 是对象 key 前缀（与图片模块建 key 的规则一致）。
	Prefix string
	// Delimiter 是七牛控制台配置的样式分隔符，默认 "-"。
	Delimiter string
	// Styles 是"用途槽位 → 样式名"。样式名为空表示该档用原图。
	Styles map[Purpose]string
	// PrivateRead 为真时所有地址都要签名（私有空间）。
	PrivateRead bool
	// GateEnabled 是审核闸门的独立开关。
	//
	// 关掉它等于"未过审也可展示"：样式照常派生，只是不再问"这张图能不能看"。
	// 留这个开关是因为审核链路（回调 + 主动查询）本身可能故障，那时需要一条
	// 不重启、不改数据就能恢复展示的退路。
	GateEnabled bool
}

// StyleOf 返回该用途配置的样式名（可能为空）。
func (c Config) StyleOf(p Purpose) string {
	if c.Styles == nil {
		return ""
	}
	return strings.TrimSpace(c.Styles[p])
}

// delimiter 返回生效的分隔符，空值回落七牛默认的 "-"。
func (c Config) delimiter() string {
	if d := strings.TrimSpace(c.Delimiter); d != "" {
		return d
	}
	return "-"
}

// Viewer 是"谁在看"。管理员能看到未过审的图（否则没法审图）。
//
// 注意：作者本人**不算**例外。规格里明确"未过审的图只有管理员能看"，
// 作者看到的是占位加一句"审核中"，避免作者把自己没过的图当成已发布。
type Viewer struct {
	IsAdmin bool
}

// Policy 是可注入的两个外部能力。为 nil 时退化成"全都可见 + 不签名"，
// 让票 411（只做样式、还没做闸门）也能复用同一份派生逻辑。
type Policy interface {
	// Visible 批量判定这批地址当前是否可对外展示。
	// 返回 map[原地址]bool；缺失的地址按"不可展示"处理。
	Visible(ctx context.Context, urls []string) (map[string]bool, error)
	// Sign 对"已经拼好样式后缀"的对象 key 签名。
	//
	// 必须传拼好样式的 key：七牛私有空间的下载凭证是按完整 URL 计算的，
	// 先对原图签名再往后加 `-style` 会让令牌失效（官方明确禁止这个顺序）。
	Sign(keyWithStyle string) string
}

// Resolver 批量派生展示地址。使用方从装配层注入，不自己构造。
type Resolver struct {
	config func() Config
	policy Policy
}

// New 构造一个派生器。config 传函数而不是值：后台保存样式名后要立刻生效。
func New(config func() Config, policy Policy) *Resolver {
	return &Resolver{config: config, policy: policy}
}

// Item 是一条待派生的地址。
type Item struct {
	URL     string
	Purpose Purpose
}

// Resolve 批量派生，返回与输入等长的结果；元素为空串表示"不要展示"。
//
// nil 接收者表示"没有装配派生能力"：此时**原样返回输入**（不改写），
// 而不是返回空串。二者差别很大：返回空串会让未装配的路径上所有图片突然消失，
// 而原样返回只是"没享受到压缩样式"，线上表现是原来的样子。
//
// 失败策略：判定接口出错时**按不可展示处理**（fail closed），并返回错误让调用方
// 决定是否记日志。宁可让图上不了，也不能让未过审的图漏出去。
func (r *Resolver) Resolve(ctx context.Context, viewer Viewer, items []Item) ([]string, error) {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = strings.TrimSpace(item.URL)
	}
	if r == nil || len(items) == 0 {
		return out, nil
	}
	cfg := r.config()

	// 先按配置把每条地址归一成 (object_key, 是否本站)。非本站的原样透传。
	keys := make([]string, len(items))
	for i := range items {
		if out[i] == "" {
			continue
		}
		key, ok := cfg.ObjectKey(out[i])
		if !ok {
			// 外链或本站其他路径：不是我们管的图片，原样返回。
			continue
		}
		keys[i] = key
	}

	// 一次查库判定可见性。
	visible := map[string]bool{}
	gate := r.policy != nil && cfg.GateEnabled && !viewer.IsAdmin
	if gate {
		urls := make([]string, 0, len(items))
		for i := range items {
			if keys[i] != "" {
				urls = append(urls, out[i])
			}
		}
		if len(urls) > 0 {
			got, err := r.policy.Visible(ctx, urls)
			if err != nil {
				// fail closed：本站图片一律不展示，但**外链原样保留** ——
				// 判定接口挂了不该顺手把人家贴的第三方图片也弄没。
				out = make([]string, len(items))
				for i := range items {
					if items[i].URL != "" && keys[i] == "" {
						out[i] = strings.TrimSpace(items[i].URL)
					}
				}
				return out, err
			}
			visible = got
		}
	}

	for i := range items {
		if out[i] == "" || keys[i] == "" {
			// 空地址与非本站地址：保持原样（外链图既不套样式也不受本站审核约束）。
			continue
		}
		if gate && !visible[items[i].URL] && !visible[out[i]] {
			out[i] = ""
			continue
		}
		out[i] = r.build(cfg, keys[i], items[i].Purpose)
	}
	return out, nil
}

// MustResolve 是 Resolve 的"调用点不关心错误"版本：出错时按**全部不可展示**处理。
//
// 为什么允许吞掉错误：调用点是一堆 DTO 映射函数，它们没有合适的错误出口
// （一个字段派生失败不该让整个列表接口 500）。而 fail closed 保证了
// 最坏结果只是"图不显示"，不会"未过审的图漏出去"。
// 需要区分错误的分支（例如探活）请直接用 Resolve。
func MustResolve(ctx context.Context, r *Resolver, viewer Viewer, items []Item) []string {
	urls, err := r.Resolve(ctx, viewer, items)
	if err != nil {
		return make([]string, len(items))
	}
	return urls
}

// build 把 object key 渲染成最终展示地址（含样式与可选签名）。
func (r *Resolver) build(cfg Config, key string, purpose Purpose) string {
	keyWithStyle := cfg.WithStyle(key, cfg.StyleOf(purpose))
	if cfg.PrivateRead && r.policy != nil {
		return r.policy.Sign(keyWithStyle)
	}
	return cfg.PublicURL(keyWithStyle)
}

// PublicURL 拼公开地址。
func (c Config) PublicURL(key string) string {
	base := strings.TrimRight(strings.TrimSpace(c.MediaBase), "/")
	if base == "" || key == "" {
		return ""
	}
	return base + "/" + strings.TrimLeft(key, "/")
}

// WithStyle 给 key 追加样式后缀。样式名为空时原样返回。
//
// 幂等：如果 key 已经带着某个**已配置**样式的尾巴，先剥掉再拼，
// 避免同一个地址被派生两次变成 `hash-small-small`（刷新页面就会发生）。
func (c Config) WithStyle(key, style string) string {
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	if key == "" {
		return ""
	}
	step := c.delimiter()
	key = c.stripStyle(key)
	style = strings.TrimSpace(style)
	if style == "" {
		return key
	}
	return key + step + style
}

// stripStyle 剥掉末尾的样式后缀。只认**已配置**的样式名：
// 对象 key 是 32 位哈希，不可能以 `-<样式名>` 结尾，而随便剥任意后缀
// 会误伤"文件名里本来就有连字符"的历史 key。
func (c Config) stripStyle(key string) string {
	step := c.delimiter()
	for _, style := range c.Styles {
		style = strings.TrimSpace(style)
		if style == "" {
			continue
		}
		suffix := step + style
		if strings.HasSuffix(key, suffix) {
			return strings.TrimSuffix(key, suffix)
		}
	}
	return key
}

// ObjectKey 从展示地址里反解出对象 key（已剥样式后缀）。
//
// 判定"是不是本站图片"靠两件事：host 与存储基址一致，且路径以 `{prefix}/images/`
// 开头（与图片模块建 key 的规则、以及引用账本反解规则保持同一口径）。
// 返回 false 表示这个地址不归我们管，调用方应原样透传。
func (c Config) ObjectKey(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || c.MediaBase == "" {
		return "", false
	}
	base, err := url.Parse(c.MediaBase)
	if err != nil || base.Host == "" {
		return "", false
	}

	var path string
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", false
		}
		if !strings.EqualFold(parsed.Host, base.Host) {
			return "", false
		}
		path = parsed.Path
	} else {
		// 相对地址（历史数据里可能有 /myblogx/images/... 这种写法）：
		// 只有在确实带本站前缀时才认，避免把任意相对路径当成本站图片。
		path = raw
		if idx := strings.IndexAny(path, "?#"); idx >= 0 {
			path = path[:idx]
		}
	}

	key := strings.TrimLeft(path, "/")
	if key == "" {
		return "", false
	}
	prefix := strings.Trim(strings.TrimSpace(c.Prefix), "/")
	if prefix == "" {
		prefix = "images"
	}
	if !strings.HasPrefix(key, prefix+"/images/") {
		return "", false
	}
	return c.stripStyle(key), true
}
