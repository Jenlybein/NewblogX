package conf

import (
	"myblogx/conf/site"
)

type Site struct {
	SiteInfo site.SiteInfo `yaml:"site_info" json:"site_info"`
	Project  site.Project  `yaml:"project" json:"project"`
	Seo      site.Seo      `yaml:"seo" json:"seo"`
	About    site.About    `yaml:"about" json:"about"`
	Login    site.Login    `yaml:"login" json:"login"`
	Article  site.Article  `yaml:"article" json:"article"`
	Comment  site.Comment  `yaml:"comment" json:"comment"`
	Chat     site.Chat     `yaml:"chat" json:"chat"`
}
