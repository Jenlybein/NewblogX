package main

import "testing"

// TestR12_ID字段必须用ctype 验证裸整型 ID 字段会被抓到、ctype.ID 与豁免不误报。
//
// 这条规则的存在意义：雪花 ID 超出 JS 安全整数，裸 uint64 会让 swag
// 把字段印成 integer，前端拿到的数字已经丢精度，而且编译期不会有任何提示。
func TestR12_ID字段必须用ctype(t *testing.T) {
	cases := []case_{
		{
			name: "裸 uint64 的 ID 字段",
			rel:  "internal/user/app/dto.go",
			src: `package app
type DetailView struct {
	UserID uint64
}
`,
			ruleID:   "R12",
			wantHits: 1,
		},
		{
			name: "指针与切片的裸整型也算",
			rel:  "internal/user/adapter/http/dto.go",
			src: `package http
type Req struct {
	AvatarImageID *uint64
	LikeTagIDs    []uint64
	TeamIDs       *[]int64
}
`,
			ruleID:   "R12",
			wantHits: 3,
		},
		{
			name: "ctype.ID 不报",
			rel:  "internal/user/app/dto.go",
			src: `package app
import "myblogx/internal/kernel/ctype"
type DetailView struct {
	UserID uint64
	ID     ctype.ID
	TagIDs []ctype.ID
}
`,
			ruleID:   "R12",
			wantHits: 1, // 只有 UserID 该报
		},
		{
			name: "名字不带 ID 语义的整数不报",
			rel:  "internal/user/app/dto.go",
			src: `package app
type Stats struct {
	Count  int
	Total  uint64
	Page   int
	Solid  uint64
}
`,
			ruleID:   "R12",
			wantHits: 0,
		},
		{
			name: "ClickHouse 落盘结构整包豁免",
			rel:  "internal/platform/log/event.go",
			src: `package log
type Event struct {
	EventID uint64
	UserID  uint64
}
`,
			ruleID:   "R12",
			wantHits: 0,
		},
		{
			name: "canal 的 server_id 豁免",
			rel:  "conf/conf_river.go",
			src: `package conf
type River struct {
	ServerID uint32
}
`,
			ruleID:   "R12",
			wantHits: 0,
		},
		{
			name: "雪花 worker 编号豁免",
			rel:  "internal/platform/idgen/idgen.go",
			src: `package idgen
var initializedID uint32
type node struct {
	initializedID uint32
	MaxID         uint64
}
`,
			ruleID:   "R12",
			wantHits: 1, // initializedID 豁免，MaxID 仍该报
		},
		{
			name: "腾讯云 SES 模板编号豁免",
			rel:  "conf/conf_mail.go",
			src: `package conf
type Mail struct {
	TemplateID uint64
	FromID     uint64
}
`,
			ruleID:   "R12",
			wantHits: 1, // TemplateID 豁免，FromID 仍该报
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			runOne(t, c)
		})
	}
}
