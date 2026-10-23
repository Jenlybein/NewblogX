package ai_metainfo

import "myblogx/models/ctype"

const (
	articleMetainfoTitleLimit    = 30
	articleMetainfoAbstractLimit = 200
	articleMetainfoMaxTags       = 3
)

// Metainfos 是文章元信息推荐时使用的候选项。
type Metainfos struct {
	ID    ctype.ID `json:"id"`
	Title string   `json:"title"`
}

// MetainfoRequest 是生成文章元信息时的请求参数。
type MetainfoRequest struct {
	UserID  ctype.ID `json:"user_id"`
	Content string   `json:"content"`
}

// MetainfoResponse 是 AI 生成并校验后的文章元信息。
type MetainfoResponse struct {
	Title    string      `json:"title"`
	Abstract string      `json:"abstract"`
	Category *Metainfos  `json:"category"`
	Tags     []Metainfos `json:"tags"`
}
