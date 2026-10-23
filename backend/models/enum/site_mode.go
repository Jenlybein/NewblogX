package enum

type SiteMode int

const (
	// 1 社区模式 2 博客模式
	SiteModeCommunity SiteMode = iota + 1
	SiteModeBlog
)
