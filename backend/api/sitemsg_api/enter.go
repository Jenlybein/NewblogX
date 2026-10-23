package sitemsg_api

import (
	"gorm.io/gorm"
)

type Deps struct {
	DB *gorm.DB
}

type SitemsgApi struct {
	App Deps
}

func New(deps Deps) SitemsgApi {
	return SitemsgApi{App: deps}
}
