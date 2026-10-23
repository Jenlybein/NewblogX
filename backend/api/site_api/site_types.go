package site_api

// 站点信息请求参数
type SiteInfoRequest struct {
	Name string `uri:"name"`
}

type SiteAIResponse struct {
	Enable   bool   `json:"enable"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Abstract string `json:"abstract"`
}

type SiteSEOResponse struct {
	SiteTitle    string `json:"site_title"`
	ProjectTitle string `json:"project_title"`
	Logo         string `json:"logo"`
	Icon         string `json:"icon"`
	Keywords     string `json:"keywords"`
	Description  string `json:"description"`
}
