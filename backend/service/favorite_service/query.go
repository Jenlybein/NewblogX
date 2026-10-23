package favorite_service

import "myblogx/repository/favor_repo"

type FavoriteListQuery = favor_repo.FavoriteListQuery
type FavoriteListItem = favor_repo.FavoriteListItem
type FavoriteArticlesQuery = favor_repo.FavoriteArticlesQuery
type FavoriteArticleItem = favor_repo.FavoriteArticleItem
type QueryService = favor_repo.QueryService

var NewQueryService = favor_repo.NewQueryService
