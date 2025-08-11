package usecases

import (
	"context"
	"errors"
	"time"

	"go-im/internal/application/ports"
	"go-im/internal/domain/entities"
)

// UserUseCase 用户用例
type UserUseCase struct {
	userRepo    ports.UserRepository
	passwordSvc ports.PasswordService
	authSvc     ports.AuthService
	idGenerator ports.IDGenerator
	presenceSvc ports.PresenceService
	metricsSvc  ports.MetricsService
	logger      ports.LogService
}

// NewUserUseCase 创建用户用例
func NewUserUseCase(
	userRepo ports.UserRepository,
	passwordSvc ports.PasswordService,
	authSvc ports.AuthService,
	idGenerator ports.IDGenerator,
	presenceSvc ports.PresenceService,
	metricsSvc ports.MetricsService,
	logger ports.LogService,
) *UserUseCase {
	return &UserUseCase{
		userRepo:    userRepo,
		passwordSvc: passwordSvc,
		authSvc:     authSvc,
		idGenerator: idGenerator,
		presenceSvc: presenceSvc,
		metricsSvc:  metricsSvc,
		logger:      logger,
	}
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
}

// RegisterResponse 注册响应
type RegisterResponse struct {
	UserID string `json:"userId"`
	Token  string `json:"token"`
}

// Register 用户注册
func (uc *UserUseCase) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	// 验证用户名是否已存在
	existingUser, err := uc.userRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		uc.logger.Error(ctx, "检查用户名失败", err, map[string]interface{}{
			"username": req.Username,
		})
		return nil, errors.New("系统错误")
	}
	if existingUser != nil {
		return nil, errors.New("用户名已存在")
	}

	// 密码加密
	hashedPassword, err := uc.passwordSvc.HashPassword(req.Password)
	if err != nil {
		uc.logger.Error(ctx, "密码加密失败", err, nil)
		return nil, errors.New("系统错误")
	}

	// 创建用户实体
	userID := uc.idGenerator.GenerateUserID()
	user, err := entities.NewUser(userID, req.Username, hashedPassword, req.Nickname)
	if err != nil {
		return nil, err
	}

	// 保存用户
	if err := uc.userRepo.Save(ctx, user); err != nil {
		uc.logger.Error(ctx, "保存用户失败", err, map[string]interface{}{
			"userId":   userID,
			"username": req.Username,
		})
		return nil, errors.New("注册失败")
	}

	// 生成访问token
	token, err := uc.authSvc.GenerateToken(ctx, userID, 24*time.Hour)
	if err != nil {
		uc.logger.Error(ctx, "生成token失败", err, map[string]interface{}{
			"userId": userID,
		})
		return nil, errors.New("注册失败")
	}

	// 记录指标
	uc.metricsSvc.IncrementCounter("users_registered_total", nil)

	uc.logger.Info(ctx, "用户注册成功", map[string]interface{}{
		"userId":   userID,
		"username": req.Username,
	})

	return &RegisterResponse{
		UserID: userID,
		Token:  token,
	}, nil
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	UserID   string            `json:"userId"`
	Username string            `json:"username"`
	Nickname string            `json:"nickname"`
	Token    string            `json:"token"`
	User     *entities.UserDTO `json:"user"`
}

// Login 用户登录
func (uc *UserUseCase) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	// 获取用户
	user, err := uc.userRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		uc.logger.Error(ctx, "查询用户失败", err, map[string]interface{}{
			"username": req.Username,
		})
		return nil, errors.New("用户名或密码错误")
	}
	if user == nil {
		return nil, errors.New("用户名或密码错误")
	}

	// 验证密码
	if !uc.passwordSvc.VerifyPassword(user.Password(), req.Password) {
		uc.logger.Info(ctx, "用户登录密码错误", map[string]interface{}{
			"userId":   user.ID(),
			"username": req.Username,
		})
		return nil, errors.New("用户名或密码错误")
	}

	// 生成访问token
	token, err := uc.authSvc.GenerateToken(ctx, user.ID(), 24*time.Hour)
	if err != nil {
		uc.logger.Error(ctx, "生成token失败", err, map[string]interface{}{
			"userId": user.ID(),
		})
		return nil, errors.New("登录失败")
	}

	// 记录指标
	uc.metricsSvc.IncrementCounter("users_login_total", nil)

	userDTO := user.ToDTO()
	uc.logger.Info(ctx, "用户登录成功", map[string]interface{}{
		"userId":   user.ID(),
		"username": req.Username,
	})

	return &LoginResponse{
		UserID:   user.ID(),
		Username: user.Username(),
		Nickname: user.Nickname(),
		Token:    token,
		User:     &userDTO,
	}, nil
}

// UpdateProfileRequest 更新资料请求
type UpdateProfileRequest struct {
	UserID    string `json:"userId"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatarUrl"`
}

// UpdateProfile 更新用户资料
func (uc *UserUseCase) UpdateProfile(ctx context.Context, req *UpdateProfileRequest) error {
	// 获取用户
	user, err := uc.userRepo.GetByID(ctx, req.UserID)
	if err != nil || user == nil {
		return errors.New("用户不存在")
	}

	// 更新资料
	user.UpdateProfile(req.Nickname, req.AvatarURL)

	// 保存更新
	if err := uc.userRepo.Update(ctx, user); err != nil {
		uc.logger.Error(ctx, "更新用户资料失败", err, map[string]interface{}{
			"userId": req.UserID,
		})
		return errors.New("更新失败")
	}

	uc.logger.Info(ctx, "用户资料更新成功", map[string]interface{}{
		"userId": req.UserID,
	})

	return nil
}

// GetUserByIDRequest 根据ID获取用户请求
type GetUserByIDRequest struct {
	UserID string `json:"userId"`
}

// GetUserByID 根据ID获取用户
func (uc *UserUseCase) GetUserByID(ctx context.Context, req *GetUserByIDRequest) (*entities.UserDTO, error) {
	user, err := uc.userRepo.GetByID(ctx, req.UserID)
	if err != nil {
		uc.logger.Error(ctx, "查询用户失败", err, map[string]interface{}{
			"userId": req.UserID,
		})
		return nil, errors.New("查询用户失败")
	}
	if user == nil {
		return nil, errors.New("用户不存在")
	}

	userDTO := user.ToDTO()
	return &userDTO, nil
}

// ListUsersRequest 获取用户列表请求
type ListUsersRequest struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

// ListUsersResponse 获取用户列表响应
type ListUsersResponse struct {
	Users []*entities.UserDTO `json:"users"`
	Total int                 `json:"total"`
}

// ListUsers 获取用户列表（管理后台使用）
func (uc *UserUseCase) ListUsers(ctx context.Context, req *ListUsersRequest) (*ListUsersResponse, error) {
	// 获取用户列表
	users, err := uc.userRepo.List(ctx, req.Offset, req.Limit)
	if err != nil {
		uc.logger.Error(ctx, "获取用户列表失败", err, map[string]interface{}{
			"offset": req.Offset,
			"limit":  req.Limit,
		})
		return nil, errors.New("获取用户列表失败")
	}

	// 获取总数
	total, err := uc.userRepo.Count(ctx)
	if err != nil {
		uc.logger.Error(ctx, "获取用户总数失败", err, nil)
		total = 0
	}

	// 转换为DTO并设置在线状态
	var userDTOs []*entities.UserDTO
	for _, user := range users {
		dto := user.ToDTO()

		// 检查在线状态
		online, err := uc.presenceSvc.IsUserOnline(ctx, user.ID())
		if err == nil {
			dto.Online = online
		}

		userDTOs = append(userDTOs, &dto)
	}

	return &ListUsersResponse{
		Users: userDTOs,
		Total: total,
	}, nil
}

// SetUserOnlineRequest 设置用户在线请求
type SetUserOnlineRequest struct {
	UserID   string `json:"userId"`
	DeviceID string `json:"deviceId"`
}

// SetUserOnline 设置用户在线状态
func (uc *UserUseCase) SetUserOnline(ctx context.Context, req *SetUserOnlineRequest) error {
	if err := uc.presenceSvc.SetUserOnline(ctx, req.UserID, req.DeviceID); err != nil {
		uc.logger.Error(ctx, "设置用户在线状态失败", err, map[string]interface{}{
			"userId":   req.UserID,
			"deviceId": req.DeviceID,
		})
		return errors.New("设置在线状态失败")
	}

	return nil
}

// SetUserOfflineRequest 设置用户离线请求
type SetUserOfflineRequest struct {
	UserID   string `json:"userId"`
	DeviceID string `json:"deviceId"`
}

// SetUserOffline 设置用户离线状态
func (uc *UserUseCase) SetUserOffline(ctx context.Context, req *SetUserOfflineRequest) error {
	if err := uc.presenceSvc.SetUserOffline(ctx, req.UserID, req.DeviceID); err != nil {
		uc.logger.Error(ctx, "设置用户离线状态失败", err, map[string]interface{}{
			"userId":   req.UserID,
			"deviceId": req.DeviceID,
		})
		return errors.New("设置离线状态失败")
	}

	return nil
}

// GetOnlineUsersCount 获取在线用户数量
func (uc *UserUseCase) GetOnlineUsersCount(ctx context.Context) (int, error) {
	onlineUsers, err := uc.presenceSvc.GetOnlineUsers(ctx)
	if err != nil {
		uc.logger.Error(ctx, "获取在线用户失败", err, nil)
		return 0, errors.New("获取在线用户数量失败")
	}

	return len(onlineUsers), nil
}
