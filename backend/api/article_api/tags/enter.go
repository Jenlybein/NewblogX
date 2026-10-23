package tags

import (
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Deps struct {
	DB     *gorm.DB
	Logger *logrus.Logger
	Redis  *redis.Client
}

type TagsApi struct {
	App Deps
}

func New(deps Deps) TagsApi {
	return TagsApi{App: deps}
}
