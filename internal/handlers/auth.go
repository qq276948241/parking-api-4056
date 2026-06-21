package handlers

import (
	"parking-system/internal/middleware"
	"parking-system/internal/models"
	"parking-system/internal/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AuthHandler struct{}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{}
}

type LoginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResp struct {
	Token        string  `json:"token"`
	AdminID      string  `json:"admin_id"`
	Username     string  `json:"username"`
	RealName     string  `json:"real_name"`
	Role         string  `json:"role"`
	ParkingLotID *string `json:"parking_lot_id"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	var admin models.Admin
	if err := utils.DB.Where("username = ?", req.Username).First(&admin).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.BadRequest(c, "用户名或密码错误")
			return
		}
		utils.InternalError(c, "查询失败")
		return
	}
	if !admin.IsActive {
		utils.BadRequest(c, "账号已被禁用")
		return
	}
	if !utils.CheckPassword(req.Password, admin.PasswordHash) {
		utils.BadRequest(c, "用户名或密码错误")
		return
	}
	token, err := utils.GenerateToken(admin.ID, admin.Username, admin.Role, admin.ParkingLotID)
	if err != nil {
		utils.InternalError(c, "生成Token失败")
		return
	}
	resp := LoginResp{
		Token:    token,
		AdminID:  admin.ID.String(),
		Username: admin.Username,
		RealName: admin.RealName,
		Role:     admin.Role,
	}
	if admin.ParkingLotID != nil {
		s := admin.ParkingLotID.String()
		resp.ParkingLotID = &s
	}
	utils.OK(c, resp)
}

func (h *AuthHandler) GetProfile(c *gin.Context) {
	adminID := middleware.GetAdminID(c)
	var admin models.Admin
	if err := utils.DB.First(&admin, adminID).Error; err != nil {
		utils.NotFound(c, "管理员不存在")
		return
	}
	utils.OK(c, gin.H{
		"id":              admin.ID,
		"username":        admin.Username,
		"real_name":       admin.RealName,
		"role":            admin.Role,
		"parking_lot_id":  admin.ParkingLotID,
		"is_active":       admin.IsActive,
	})
}

type ChangePwdReq struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req ChangePwdReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	adminID := middleware.GetAdminID(c)
	var admin models.Admin
	if err := utils.DB.First(&admin, adminID).Error; err != nil {
		utils.NotFound(c, "管理员不存在")
		return
	}
	if !utils.CheckPassword(req.OldPassword, admin.PasswordHash) {
		utils.BadRequest(c, "原密码错误")
		return
	}
	hash, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		utils.InternalError(c, "密码加密失败")
		return
	}
	admin.PasswordHash = hash
	if err := utils.DB.Save(&admin).Error; err != nil {
		utils.InternalError(c, "修改密码失败")
		return
	}
	utils.OK(c, nil)
}
