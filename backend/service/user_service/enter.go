package user_service

import "myblogx/models"

type UserService struct {
	userModel models.UserModel
}

func NewUserService(user models.UserModel) *UserService {
	return &UserService{
		userModel: user,
	}
}
