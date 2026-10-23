package article_api

import (
	"myblogx/api/article_api/category"
	"myblogx/api/article_api/favorite"
	"myblogx/api/article_api/tags"
	"myblogx/api/article_api/top"
	"myblogx/api/article_api/view_history"
	"myblogx/conf"
	"myblogx/service/site_service"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Deps struct {
	DB          *gorm.DB
	JWT         conf.Jwt
	Logger      *logrus.Logger
	Redis       *redis.Client
	RuntimeSite *site_service.RuntimeConfigService
}

type ArticleApi struct {
	App Deps
	category.CategoryApi
	favorite.FavoriteApi
	top.TopApi
	view_history.ViewHistoryApi
	tags.TagsApi
}

func New(deps Deps) ArticleApi {
	return ArticleApi{
		App: deps,
		CategoryApi: category.New(category.Deps{
			DB:     deps.DB,
			JWT:    deps.JWT,
			Logger: deps.Logger,
			Redis:  deps.Redis,
		}),
		FavoriteApi: favorite.New(favorite.Deps{
			DB:     deps.DB,
			JWT:    deps.JWT,
			Logger: deps.Logger,
			Redis:  deps.Redis,
		}),
		TopApi: top.New(top.Deps{
			DB:     deps.DB,
			Logger: deps.Logger,
			Redis:  deps.Redis,
		}),
		ViewHistoryApi: view_history.New(view_history.Deps{
			DB: deps.DB,
		}),
		TagsApi: tags.New(tags.Deps{
			DB:     deps.DB,
			Logger: deps.Logger,
			Redis:  deps.Redis,
		}),
	}
}
