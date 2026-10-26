package imageref

// replayPayload 是 image_ref_river 死信的 payload 形状（三种动作的字段并集）。
//
// # 为什么写入端与重投端必须共用这一个类型
//
// 之前写入端在 OnRow 里拼 `map[string]any`，重投端另写一个结构体，两边靠
// "字段名恰好一致"对齐 —— 这种对齐没有任何编译期保证。有人把 owner_id 改成
// ownerId，重投就会一律报"只支持删除类死信"，而单元测试因为手写 JSON 字符串
// 仍然全绿。收成一个类型之后，改名会在**写入端的构造处**直接编译失败。
//
// 这是本项目反复踩过的同一类坑（见 12 §7.1「注解写在死代码上」）：
// 两份独立的事实 + 没有机械对账 = 静默漂移。能靠编译器解决的，别靠测试。
//
// # 三种动作的字段是互斥的
//
//	删除：owner_id + ref_type          → 重投 = 重新清理该对象的图片引用
//	新增：table + row                  → 重投 = 不支持（行数据经 JSON 往返后类型不可靠）
//	更新：table + before + after       → 同上
type replayPayload struct {
	OwnerID string `json:"owner_id,omitempty"`
	RefType string `json:"ref_type,omitempty"`
	Table   string `json:"table,omitempty"`
	Row     []any  `json:"row,omitempty"`
	Before  []any  `json:"before,omitempty"`
	After   []any  `json:"after,omitempty"`
}

// isDelete 判断是否为删除类（payload 自洽，可以重投）。
func (p replayPayload) isDelete() bool { return p.OwnerID != "" && p.RefType != "" }

// isRowBased 判断是否为新增/更新类（带着 binlog 原始行，不能重投）。
//
// 单独判断它、而不是只判断"不是删除类就报错"，是为了把两种情况分清楚：
//   - 新增/更新类 → ErrUnsupported（语义上不支持，人该去重新保存对象）；
//   - 连形状都认不出来 → 普通 error（payload 坏了，属于真故障，需要人看）。
func (p replayPayload) isRowBased() bool {
	return p.Table != "" && (len(p.Row) > 0 || len(p.After) > 0)
}
