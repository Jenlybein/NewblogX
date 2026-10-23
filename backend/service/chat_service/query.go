package chat_service

import "myblogx/repository/chat_repo"

type SessionListItem = chat_repo.SessionListItem
type SessionListQuery = chat_repo.SessionListQuery
type QueryService = chat_repo.QueryService

var NewQueryService = chat_repo.NewQueryService
