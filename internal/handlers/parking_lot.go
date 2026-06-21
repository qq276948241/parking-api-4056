package handlers

import (
	"parking-system/internal/middleware"
	"parking-system/internal/models"
	"parking-system/internal/utils"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ParkingLotHandler struct{}

func NewParkingLotHandler() *ParkingLotHandler {
	return &ParkingLotHandler{}
}

type LotCreateReq struct {
	Name         string  `json:"name" binding:"required"`
	Address      string  `json:"address"`
	ContactPhone string  `json:"contact_phone"`
	TotalSpaces  int     `json:"total_spaces" binding:"required,min=1"`
	HourlyRate   float64 `json:"hourly_rate" binding:"required,min=0"`
	DailyMax     float64 `json:"daily_max" binding:"min=0"`
	FreeMinutes  int     `json:"free_minutes" binding:"min=0"`
	IsActive     *bool   `json:"is_active"`
}

type LotUpdateReq struct {
	Name         string   `json:"name"`
	Address      string   `json:"address"`
	ContactPhone string   `json:"contact_phone"`
	TotalSpaces  *int     `json:"total_spaces" binding:"omitempty,min=1"`
	HourlyRate   *float64 `json:"hourly_rate" binding:"omitempty,min=0"`
	DailyMax     *float64 `json:"daily_max" binding:"omitempty,min=0"`
	FreeMinutes  *int     `json:"free_minutes" binding:"omitempty,min=0"`
	IsActive     *bool    `json:"is_active"`
}

func (h *ParkingLotHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	keyword := c.Query("keyword")
	isActive := c.Query("is_active")

	db := utils.DB.Model(&models.ParkingLot{})
	if !middleware.IsSuperAdmin(c) {
		pid := middleware.GetParkingLotID(c)
		if pid != nil {
			db = db.Where("id = ?", *pid)
		}
	}
	if keyword != "" {
		db = db.Where("name LIKE ? OR address LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if isActive != "" {
		active := isActive == "true" || isActive == "1"
		db = db.Where("is_active = ?", active)
	}

	var total int64
	db.Count(&total)
	var list []models.ParkingLot
	db.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list)
	utils.OKPaged(c, list, total, page, pageSize)
}

func (h *ParkingLotHandler) Create(c *gin.Context) {
	if !middleware.IsSuperAdmin(c) {
		utils.Forbidden(c, "需要超级管理员权限")
		return
	}
	var req LotCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	lot := models.ParkingLot{
		Name:         req.Name,
		Address:      req.Address,
		ContactPhone: req.ContactPhone,
		TotalSpaces:  req.TotalSpaces,
		HourlyRate:   req.HourlyRate,
		DailyMax:     req.DailyMax,
		FreeMinutes:  req.FreeMinutes,
		IsActive:     true,
	}
	if req.IsActive != nil {
		lot.IsActive = *req.IsActive
	}
	if err := utils.DB.Create(&lot).Error; err != nil {
		utils.InternalError(c, "创建停车场失败: "+err.Error())
		return
	}
	utils.OK(c, lot)
}

func (h *ParkingLotHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	var lot models.ParkingLot
	if err := utils.DB.First(&lot, id).Error; err != nil {
		utils.NotFound(c, "停车场不存在")
		return
	}
	if !middleware.IsSuperAdmin(c) {
		pid := middleware.GetParkingLotID(c)
		if pid == nil || lot.ID != *pid {
			utils.Forbidden(c, "无权访问该停车场")
			return
		}
	}
	utils.OK(c, lot)
}

func (h *ParkingLotHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	if !middleware.IsSuperAdmin(c) {
		pid := middleware.GetParkingLotID(c)
		if pid == nil || id != *pid {
			utils.Forbidden(c, "无权修改该停车场")
			return
		}
	}
	var lot models.ParkingLot
	if err := utils.DB.First(&lot, id).Error; err != nil {
		utils.NotFound(c, "停车场不存在")
		return
	}
	var req LotUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if req.Name != "" {
		lot.Name = req.Name
	}
	if req.Address != "" {
		lot.Address = req.Address
	}
	if req.ContactPhone != "" {
		lot.ContactPhone = req.ContactPhone
	}
	if req.TotalSpaces != nil {
		lot.TotalSpaces = *req.TotalSpaces
	}
	if req.HourlyRate != nil {
		lot.HourlyRate = *req.HourlyRate
	}
	if req.DailyMax != nil {
		lot.DailyMax = *req.DailyMax
	}
	if req.FreeMinutes != nil {
		lot.FreeMinutes = *req.FreeMinutes
	}
	if req.IsActive != nil {
		lot.IsActive = *req.IsActive
	}
	if err := utils.DB.Save(&lot).Error; err != nil {
		utils.InternalError(c, "更新停车场失败")
		return
	}
	utils.OK(c, lot)
}

func (h *ParkingLotHandler) Delete(c *gin.Context) {
	if !middleware.IsSuperAdmin(c) {
		utils.Forbidden(c, "需要超级管理员权限")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	result := utils.DB.Delete(&models.ParkingLot{}, id)
	if result.Error != nil {
		utils.InternalError(c, "删除停车场失败")
		return
	}
	if result.RowsAffected == 0 {
		utils.NotFound(c, "停车场不存在")
		return
	}
	utils.OK(c, nil)
}

type LotStats struct {
	TotalSpaces    int `json:"total_spaces"`
	AvailableSpaces int `json:"available_spaces"`
	OccupiedSpaces int `json:"occupied_spaces"`
	CurrentParking int `json:"current_parking"`
	TodayEntry     int64 `json:"today_entry"`
	TodayExit      int64 `json:"today_exit"`
	TodayIncome    float64 `json:"today_income"`
}

func (h *ParkingLotHandler) Stats(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	if !middleware.IsSuperAdmin(c) {
		pid := middleware.GetParkingLotID(c)
		if pid == nil || id != *pid {
			utils.Forbidden(c, "无权访问该停车场")
			return
		}
	}
	var lot models.ParkingLot
	if err := utils.DB.First(&lot, id).Error; err != nil {
		utils.NotFound(c, "停车场不存在")
		return
	}

	stats := LotStats{TotalSpaces: lot.TotalSpaces}
	utils.DB.Model(&models.ParkingSpace{}).Where("parking_lot_id = ? AND status = ?", id, "available").Count(new(int64))

	var availCount int64
	utils.DB.Model(&models.ParkingSpace{}).Where("parking_lot_id = ? AND status = ?", id, "available").Count(&availCount)
	stats.AvailableSpaces = int(availCount)

	var occupiedCount int64
	utils.DB.Model(&models.ParkingSpace{}).Where("parking_lot_id = ? AND status = ?", id, "occupied").Count(&occupiedCount)
	stats.OccupiedSpaces = int(occupiedCount)

	var parkingCount int64
	utils.DB.Model(&models.ParkingRecord{}).Where("parking_lot_id = ? AND status = ?", id, "parking").Count(&parkingCount)
	stats.CurrentParking = int(parkingCount)

	today := "DATE(created_at) = CURRENT_DATE"
	var todayEntry int64
	utils.DB.Model(&models.ParkingRecord{}).Where("parking_lot_id = ? AND "+today, id).Count(&todayEntry)
	stats.TodayEntry = todayEntry

	var todayExit int64
	utils.DB.Model(&models.ParkingRecord{}).Where("parking_lot_id = ? AND status = ? AND DATE(exit_time) = CURRENT_DATE", id, "completed").Count(&todayExit)
	stats.TodayExit = todayExit

	type incomeRow struct {
		Sum float64
	}
	var ir incomeRow
	utils.DB.Model(&models.PaymentRecord{}).Select("COALESCE(SUM(amount),0) as sum").
		Where("parking_lot_id = ? AND status = ? AND payment_type = ? AND DATE(created_at) = CURRENT_DATE", id, "success", "parking").
		Scan(&ir)
	stats.TodayIncome = ir.Sum

	utils.OK(c, stats)
}
