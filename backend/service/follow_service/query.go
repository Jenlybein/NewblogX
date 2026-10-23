package follow_service

import "myblogx/repository/follow_repo"

type FollowListItem = follow_repo.FollowListItem
type FansListItem = follow_repo.FansListItem
type QueryService = follow_repo.QueryService

var NewQueryService = follow_repo.NewQueryService
