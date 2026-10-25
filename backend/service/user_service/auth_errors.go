package user_service

import "myblogx/internal/apperr"

var (
	ErrAuthRequired         = apperr.New(apperr.CodeAuthRequired)
	ErrAuthInvalid          = apperr.New(apperr.CodeAuthTokenExpired)
	ErrLoginTooFrequent     = apperr.New(apperr.CodeAuthTooFrequent)
	ErrSendTooFrequent      = apperr.New(apperr.CodeAuthSendTooFrequent)
	ErrRefreshTokenRequired = apperr.New(apperr.CodeAuthRefreshMissing)
	ErrRefreshTokenInvalid  = apperr.New(apperr.CodeAuthRefreshInvalid)
)
