package view_history

import (
	"gorm.io/gorm"
)

type Deps struct {
	DB *gorm.DB
}

type ViewHistoryApi struct {
	App Deps
}

func New(deps Deps) ViewHistoryApi {
	return ViewHistoryApi{App: deps}
}
