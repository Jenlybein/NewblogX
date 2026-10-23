package follow_api

import (
	"gorm.io/gorm"
)

type Deps struct {
	DB *gorm.DB
}

type FollowApi struct {
	App Deps
}

func New(deps Deps) FollowApi {
	return FollowApi{App: deps}
}
