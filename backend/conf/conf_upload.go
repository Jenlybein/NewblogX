// 上传策略配置
//
// 只有 Whitelist 真正生效：媒体模块按文件名后缀与探测到的真实格式双向校验。
// 历史上还有 Size 与 UploadDir 两个字段，全仓零读取（settings.yaml 的注释
// 也自称"预留/兼容字段"），已删除 —— 上传大小真正生效的是七牛的 size，
// 静态目录则硬编码在 router 的 r.Static("/uploads", "./uploads")。
package conf

// Upload 是上传策略。
type Upload struct {
	// Whitelist 允许上传的图片后缀；前后端与格式探测都会用它
	Whitelist []string `yaml:"whitelist" json:"whitelist"`
}
