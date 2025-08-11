package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"go-im/internal/application/usecases"
)

// UserHandler 用户HTTP处理器
type UserHandler struct {
	userUseCase *usecases.UserUseCase
}

// NewUserHandler 创建用户HTTP处理器
func NewUserHandler(userUseCase *usecases.UserUseCase) *UserHandler {
	return &UserHandler{
		userUseCase: userUseCase,
	}
}

// Register 用户注册
func (h *UserHandler) Register(c *gin.Context) {
	var req usecases.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.userUseCase.Register(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, resp)
}

// Login 用户登录
func (h *UserHandler) Login(c *gin.Context) {
	var req usecases.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.userUseCase.Login(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// UpdateProfile 更新用户资料
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID := c.GetString("userID") // 从中间件获取用户ID
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	var req usecases.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.UserID = userID
	err := h.userUseCase.UpdateProfile(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// GetProfile 获取用户资料
func (h *UserHandler) GetProfile(c *gin.Context) {
	userID := c.GetString("userID")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	req := &usecases.GetUserByIDRequest{UserID: userID}
	user, err := h.userUseCase.GetUserByID(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}

// ListUsers 获取用户列表（管理员功能）
func (h *UserHandler) ListUsers(c *gin.Context) {
	offsetStr := c.DefaultQuery("offset", "0")
	limitStr := c.DefaultQuery("limit", "20")

	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的offset参数"})
		return
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的limit参数"})
		return
	}

	req := &usecases.ListUsersRequest{
		Offset: offset,
		Limit:  limit,
	}

	resp, err := h.userUseCase.ListUsers(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// SetOnlineStatus 设置在线状态
func (h *UserHandler) SetOnlineStatus(c *gin.Context) {
	userID := c.GetString("userID")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	deviceID := c.PostForm("deviceId")
	if deviceID == "" {
		deviceID = "default"
	}

	status := c.PostForm("status") // "online" or "offline"

	if status == "online" {
		req := &usecases.SetUserOnlineRequest{
			UserID:   userID,
			DeviceID: deviceID,
		}
		err := h.userUseCase.SetUserOnline(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else if status == "offline" {
		req := &usecases.SetUserOfflineRequest{
			UserID:   userID,
			DeviceID: deviceID,
		}
		err := h.userUseCase.SetUserOffline(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的状态"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "状态更新成功"})
}
