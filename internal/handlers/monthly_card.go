package handlers

import (
	"parking-system/internal/middleware"
	"parking-system/internal/models"
	"parking-system/internal/service"
	"parking-system/internal/utils"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type MonthlyCardHandler struct {
	OpsSvc  *service.MonthlyCardOpsService
	CardSvc *service.MonthlyCardService
}

func NewMonthlyCardHandler() *MonthlyCardHandler {
	ops := service.NewMonthlyCardOpsService()
	return &MonthlyCardHandler{
		OpsSvc:  ops,
		CardSvc: ops.CardSvc,
	}
}

type CardCreateReq struct {
	ParkingLotID  string  `json:"parking_lot_id"`
	CardNumber    string  `json:"card_number"`
	VehiclePlate  string  `json:"vehicle_plate" binding:"required"`
	OwnerName     string  `json:"owner_name"`
	OwnerPhone    string  `json:"owner_phone"`
	PlanName      string  `json:"plan_name" binding:"required"`
	PlanType      string  `json:"plan_type" binding:"required,oneof=monthly quarterly yearly"`
	Price         float64 `json:"price" binding:"required,min=0"`
	StartDate     string  `json:"start_date" binding:"required"`
	Months        int     `json:"months"`
	EndDate       string  `json:"end_date"`
	PaidAmount    float64 `json:"paid_amount" binding:"min=0"`
	PaymentMethod string  `json:"payment_method" binding:"omitempty,oneof=cash wechat alipay card"`
	TransactionNo string  `json:"transaction_no"`
	Remarks       string  `json:"remarks"`
}

type CardUpdateReq struct {
	CardNumber    string   `json:"card_number"`
	VehiclePlate  string   `json:"vehicle_plate"`
	OwnerName     string   `json:"owner_name"`
	OwnerPhone    string   `json:"owner_phone"`
	PlanName      string   `json:"plan_name"`
	PlanType      string   `json:"plan_type" binding:"omitempty,oneof=monthly quarterly yearly"`
	Price         *float64 `json:"price" binding:"omitempty,min=0"`
	StartDate     *string  `json:"start_date"`
	EndDate       *string  `json:"end_date"`
	Status        string   `json:"status" binding:"omitempty,oneof=active expired suspended"`
	PaidAmount    *float64 `json:"paid_amount" binding:"omitempty,min=0"`
	PaymentMethod string   `json:"payment_method" binding:"omitempty,oneof=cash wechat alipay card"`
	Remarks       string   `json:"remarks"`
}

type CardRenewReq struct {
	PlanType      string  `json:"plan_type" binding:"required,oneof=monthly quarterly yearly"`
	Months        int     `json:"months" binding:"required,min=1"`
	Price         float64 `json:"price" binding:"required,min=0"`
	PaidAmount    float64 `json:"paid_amount" binding:"required,min=0"`
	PaymentMethod string  `json:"payment_method" binding:"required,oneof=cash wechat alipay card"`
	TransactionNo string  `json:"transaction_no"`
	StartFromNow  bool    `json:"start_from_now"`
}

func (h *MonthlyCardHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	keyword := c.Query("keyword")
	plate := c.Query("vehicle_plate")
	status := c.Query("status")
	planType := c.Query("plan_type")
	expiring := c.Query("expiring_days")

	db := utils.DB.Model(&models.MonthlyCard{})
	db = scopedLotID(c, db, "parking_lot_id")

	if keyword != "" {
		db = db.Where("card_number LIKE ? OR owner_name LIKE ? OR owner_phone LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	if plate != "" {
		db = db.Where("vehicle_plate LIKE ?", "%"+plate+"%")
	}
	if status != "" {
		db = db.Where("status = ?", status)
	}
	if planType != "" {
		db = db.Where("plan_type = ?", planType)
	}
	if expiring != "" {
		days, _ := strconv.Atoi(expiring)
		if days > 0 {
			target := time.Now().AddDate(0, 0, days)
			db = db.Where("end_date <= ? AND status = ?", target, "active")
		}
	}

	var total int64
	db.Count(&total)
	var list []models.MonthlyCard
	db.Order("end_date DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list)
	utils.OKPaged(c, list, total, page, pageSize)
}

func (h *MonthlyCardHandler) Create(c *gin.Context) {
	lotID, ok := getTargetLotID(c)
	if !ok {
		return
	}
	var req CardCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	startDate, err := time.ParseInLocation("2006-01-02", req.StartDate, time.Local)
	if err != nil {
		utils.BadRequest(c, "开始日期格式错误，应为YYYY-MM-DD")
		return
	}

	var endDatePtr *time.Time
	if req.EndDate != "" {
		endDate, err := time.ParseInLocation("2006-01-02", req.EndDate, time.Local)
		if err != nil {
			utils.BadRequest(c, "结束日期格式错误，应为YYYY-MM-DD")
			return
		}
		endDatePtr = &endDate
	}

	opID := middleware.GetAdminID(c)
	result, err := h.OpsSvc.CreateCard(service.CreateCardParams{
		ParkingLotID:  lotID,
		CardNumber:    req.CardNumber,
		VehiclePlate:  req.VehiclePlate,
		OwnerName:     req.OwnerName,
		OwnerPhone:    req.OwnerPhone,
		PlanName:      req.PlanName,
		PlanType:      req.PlanType,
		Price:         req.Price,
		StartDate:     startDate,
		Months:        req.Months,
		EndDate:       endDatePtr,
		PaidAmount:    req.PaidAmount,
		PaymentMethod: req.PaymentMethod,
		TransactionNo: req.TransactionNo,
		OperatorID:    opID,
		Remarks:       req.Remarks,
	})
	if err != nil {
		if err == service.ErrInvalidDateRange {
			utils.BadRequest(c, err.Error())
		} else {
			utils.InternalError(c, "创建月卡失败，卡号可能已存在")
		}
		return
	}

	utils.OK(c, result.Card)
}

func (h *MonthlyCardHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	var card models.MonthlyCard
	if err := utils.DB.First(&card, id).Error; err != nil {
		utils.NotFound(c, "月卡不存在")
		return
	}
	if err := checkLotAccess(c, card.ParkingLotID); err != nil {
		utils.Forbidden(c, err.Error())
		return
	}

	cardPtr, records, validity, err := h.OpsSvc.GetCardWithRecords(id)
	if err != nil {
		if err == service.ErrCardNotFound {
			utils.NotFound(c, "月卡不存在")
		} else {
			utils.InternalError(c, "查询失败")
		}
		return
	}

	utils.OK(c, gin.H{
		"card":           cardPtr,
		"is_active":      validity.Valid,
		"remaining_days": validity.RemainingDays,
		"remaining_hours": validity.RemainingHours,
		"valid_reason":   validity.Reason,
		"recent_records": records,
	})
}

func (h *MonthlyCardHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	var card models.MonthlyCard
	if err := utils.DB.First(&card, id).Error; err != nil {
		utils.NotFound(c, "月卡不存在")
		return
	}
	if err := checkLotAccess(c, card.ParkingLotID); err != nil {
		utils.Forbidden(c, err.Error())
		return
	}
	var req CardUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if req.CardNumber != "" {
		card.CardNumber = req.CardNumber
	}
	if req.VehiclePlate != "" {
		card.VehiclePlate = req.VehiclePlate
	}
	if req.OwnerName != "" {
		card.OwnerName = req.OwnerName
	}
	if req.OwnerPhone != "" {
		card.OwnerPhone = req.OwnerPhone
	}
	if req.PlanName != "" {
		card.PlanName = req.PlanName
	}
	if req.PlanType != "" {
		card.PlanType = req.PlanType
	}
	if req.Price != nil {
		card.Price = *req.Price
	}
	if req.StartDate != nil {
		if t, err := time.ParseInLocation("2006-01-02", *req.StartDate, time.Local); err == nil {
			card.StartDate = t
		}
	}
	if req.EndDate != nil {
		if t, err := time.ParseInLocation("2006-01-02", *req.EndDate, time.Local); err == nil {
			card.EndDate = t
		}
	}
	if req.Status != "" {
		card.Status = req.Status
	}
	if req.PaidAmount != nil {
		card.PaidAmount = *req.PaidAmount
	}
	if req.PaymentMethod != "" {
		card.PaymentMethod = req.PaymentMethod
	}
	if req.Remarks != "" {
		card.Remarks = req.Remarks
	}
	if card.Status == "active" && card.EndDate.Before(time.Now()) {
		card.Status = "expired"
	}
	if err := utils.DB.Save(&card).Error; err != nil {
		utils.InternalError(c, "更新月卡失败")
		return
	}
	utils.OK(c, card)
}

func (h *MonthlyCardHandler) Renew(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	var card models.MonthlyCard
	if err := utils.DB.First(&card, id).Error; err != nil {
		utils.NotFound(c, "月卡不存在")
		return
	}
	if err := checkLotAccess(c, card.ParkingLotID); err != nil {
		utils.Forbidden(c, err.Error())
		return
	}
	var req CardRenewReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	opID := middleware.GetAdminID(c)
	result, err := h.OpsSvc.RenewCard(service.RenewCardParams{
		CardID:        id,
		PlanType:      req.PlanType,
		Months:        req.Months,
		Price:         req.Price,
		PaidAmount:    req.PaidAmount,
		PaymentMethod: req.PaymentMethod,
		TransactionNo: req.TransactionNo,
		OperatorID:    opID,
		StartFromNow:  req.StartFromNow,
	})
	if err != nil {
		if err == service.ErrCardNotFound {
			utils.NotFound(c, "月卡不存在")
		} else if err == service.ErrInvalidDateRange {
			utils.BadRequest(c, err.Error())
		} else {
			utils.InternalError(c, "续费失败")
		}
		return
	}

	utils.OK(c, result.Card)
}

func (h *MonthlyCardHandler) Check(c *gin.Context) {
	plate := c.Query("vehicle_plate")
	if plate == "" {
		utils.BadRequest(c, "缺少车牌号参数")
		return
	}
	lotID, ok := getTargetLotID(c)
	if !ok {
		return
	}

	card, err := h.CardSvc.CheckVehicleActiveCard(lotID, plate)
	if err != nil {
		utils.OK(c, gin.H{
			"has_card":       false,
			"is_active":      false,
			"remaining_days": 0,
		})
		return
	}
	validity := h.CardSvc.GetValidity(card)
	utils.OK(c, gin.H{
		"has_card":       true,
		"is_active":      validity.Valid,
		"card":           card,
		"remaining_days": validity.RemainingDays,
		"remaining_hours": validity.RemainingHours,
	})
}

func (h *MonthlyCardHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	var card models.MonthlyCard
	if err := utils.DB.First(&card, id).Error; err != nil {
		utils.NotFound(c, "月卡不存在")
		return
	}
	if err := checkLotAccess(c, card.ParkingLotID); err != nil {
		utils.Forbidden(c, err.Error())
		return
	}
	result := utils.DB.Delete(&models.MonthlyCard{}, id)
	if result.Error != nil {
		utils.InternalError(c, "删除月卡失败")
		return
	}
	utils.OK(c, nil)
}
