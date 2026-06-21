package handlers

import (
	"parking-system/internal/middleware"
	"parking-system/internal/models"
	"parking-system/internal/utils"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AdminHandler struct{}

func NewAdminHandler() *AdminHandler {
	return &AdminHandler{}
}

type AdminCreateReq struct {
	Username     string `json:"username" binding:"required,min=3,max=50"`
	Password     string `json:"password" binding:"required,min=6"`
	RealName     string `json:"real_name"`
	Role         string `json:"role" binding:"required,oneof=super_admin admin"`
	ParkingLotID string `json:"parking_lot_id"`
	IsActive     *bool  `json:"is_active"`
}

type AdminUpdateReq struct {
	RealName     string `json:"real_name"`
	Role         string `json:"role" binding:"omitempty,oneof=super_admin admin"`
	ParkingLotID *string `json:"parking_lot_id"`
	IsActive     *bool  `json:"is_active"`
	Password     string `json:"password"`
}

func (h *AdminHandler) List(c *gin.Context) {
	if !middleware.IsSuperAdmin(c) {
		utils.Forbidden(c, "需要超级管理员权限")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	keyword := c.Query("keyword")
	role := c.Query("role")

	db := utils.DB.Model(&models.Admin{})
	if keyword != "" {
		db = db.Where("username LIKE ? OR real_name LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if role != "" {
		db = db.Where("role = ?", role)
	}
	var total int64
	db.Count(&total)

	var list []models.Admin
	db.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list)

	utils.OKPaged(c, list, total, page, pageSize)
}

func (h *AdminHandler) Create(c *gin.Context) {
	if !middleware.IsSuperAdmin(c) {
		utils.Forbidden(c, "需要超级管理员权限")
		return
	}
	var req AdminCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	var existing models.Admin
	if err := utils.DB.Where("username = ?", req.Username).First(&existing).Error; err == nil {
		utils.BadRequest(c, "用户名已存在")
		return
	}
	hash, err := utils.HashPassword(req.Password)
	if err != nil {
		utils.InternalError(c, "密码加密失败")
		return
	}
	admin := models.Admin{
		Username:     req.Username,
		PasswordHash: hash,
		RealName:     req.RealName,
		Role:         req.Role,
		IsActive:     true,
	}
	if req.IsActive != nil {
		admin.IsActive = *req.IsActive
	}
	if req.Role == "admin" && req.ParkingLotID != "" {
		pid, err := uuid.Parse(req.ParkingLotID)
		if err != nil {
			utils.BadRequest(c, "停车场ID格式错误")
			return
		}
		admin.ParkingLotID = &pid
	}
	if err := utils.DB.Create(&admin).Error; err != nil {
		utils.InternalError(c, "创建管理员失败: "+err.Error())
		return
	}
	utils.OK(c, admin)
}

func (h *AdminHandler) Get(c *gin.Context) {
	if !middleware.IsSuperAdmin(c) {
		utils.Forbidden(c, "需要超级管理员权限")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	var admin models.Admin
	if err := utils.DB.First(&admin, id).Error; err != nil {
		utils.NotFound(c, "管理员不存在")
		return
	}
	utils.OK(c, admin)
}

func (h *AdminHandler) Update(c *gin.Context) {
	if !middleware.IsSuperAdmin(c) {
		utils.Forbidden(c, "需要超级管理员权限")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	var admin models.Admin
	if err := utils.DB.First(&admin, id).Error; err != nil {
		utils.NotFound(c, "管理员不存在")
		return
	}
	var req AdminUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if req.RealName != "" {
		admin.RealName = req.RealName
	}
	if req.Role != "" {
		admin.Role = req.Role
	}
	if req.IsActive != nil {
		admin.IsActive = *req.IsActive
	}
	if req.Password != "" {
		hash, err := utils.HashPassword(req.Password)
		if err != nil {
			utils.InternalError(c, "密码加密失败")
			return
		}
		admin.PasswordHash = hash
	}
	if req.Role == "super_admin" {
		admin.ParkingLotID = nil
	} else if req.ParkingLotID != nil {
		if *req.ParkingLotID == "" {
			admin.ParkingLotID = nil
		} else {
			pid, err := uuid.Parse(*req.ParkingLotID)
			if err != nil {
				utils.BadRequest(c, "停车场ID格式错误")
				return
			}
			admin.ParkingLotID = &pid
		}
	}
	if err := utils.DB.Save(&admin).Error; err != nil {
		utils.InternalError(c, "更新管理员失败")
		return
	}
	utils.OK(c, admin)
}

func (h *AdminHandler) Delete(c *gin.Context) {
	if !middleware.IsSuperAdmin(c) {
		utils.Forbidden(c, "需要超级管理员权限")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	result := utils.DB.Delete(&models.Admin{}, id)
	if result.Error != nil {
		utils.InternalError(c, "删除管理员失败")
		return
	}
	if result.RowsAffected == 0 {
		utils.NotFound(c, "管理员不存在")
		return
	}
	utils.OK(c, nil)
}

func scopedLotID(c *gin.Context, db *gorm.DB, lotField string) *gorm.DB {
	if middleware.IsSuperAdmin(c) {
		lid := c.Query("parking_lot_id")
		if lid != "" {
			if pid, err := uuid.Parse(lid); err == nil {
				return db.Where(lotField+" = ?", pid)
			}
		}
		return db
	}
	pid := middleware.GetParkingLotID(c)
	if pid == nil {
		return db.Where("1=0")
	}
	return db.Where(lotField+" = ?", *pid)
}
