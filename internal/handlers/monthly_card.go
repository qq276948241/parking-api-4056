package handlers

import (
	"fmt"
	"parking-system/internal/middleware"
	"parking-system/internal/models"
	"parking-system/internal/utils"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type MonthlyCardHandler struct{}

func NewMonthlyCardHandler() *MonthlyCardHandler {
	return &MonthlyCardHandler{}
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
	CardNumber    string  `json:"card_number"`
	VehiclePlate  string  `json:"vehicle_plate"`
	OwnerName     string  `json:"owner_name"`
	OwnerPhone    string  `json:"owner_phone"`
	PlanName      string  `json:"plan_name"`
	PlanType      string  `json:"plan_type" binding:"omitempty,oneof=monthly quarterly yearly"`
	Price         *float64 `json:"price" binding:"omitempty,min=0"`
	StartDate     *string `json:"start_date"`
	EndDate       *string `json:"end_date"`
	Status        string  `json:"status" binding:"omitempty,oneof=active expired suspended"`
	PaidAmount    *float64 `json:"paid_amount" binding:"omitempty,min=0"`
	PaymentMethod string  `json:"payment_method" binding:"omitempty,oneof=cash wechat alipay card"`
	Remarks       string  `json:"remarks"`
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

	var endDate time.Time
	if req.EndDate != "" {
		endDate, err = time.ParseInLocation("2006-01-02", req.EndDate, time.Local)
		if err != nil {
			utils.BadRequest(c, "结束日期格式错误，应为YYYY-MM-DD")
			return
		}
	} else {
		months := req.Months
		if months <= 0 {
			switch req.PlanType {
			case "monthly":
				months = 1
			case "quarterly":
				months = 3
			case "yearly":
				months = 12
			}
		}
		endDate = startDate.AddDate(0, months, -1)
	}
	if endDate.Before(startDate) {
		utils.BadRequest(c, "结束日期不能早于开始日期")
		return
	}

	cardNumber := req.CardNumber
	if cardNumber == "" {
		cardNumber = fmt.Sprintf("MC%s%s", lotID.String()[:8], time.Now().Format("20060102150405"))
	}

	card := models.MonthlyCard{
		ParkingLotID:  lotID,
		CardNumber:    cardNumber,
		VehiclePlate:  req.VehiclePlate,
		OwnerName:     req.OwnerName,
		OwnerPhone:    req.OwnerPhone,
		PlanName:      req.PlanName,
		PlanType:      req.PlanType,
		Price:         req.Price,
		StartDate:     startDate,
		EndDate:       endDate,
		Status:        "active",
		PaidAmount:    req.PaidAmount,
		PaymentMethod: req.PaymentMethod,
		Remarks:       req.Remarks,
	}
	if endDate.Before(time.Now()) {
		card.Status = "expired"
	}

	tx := utils.DB.Begin()
	if err := tx.Create(&card).Error; err != nil {
		tx.Rollback()
		utils.InternalError(c, "创建月卡失败，卡号可能已存在")
		return
	}
	if req.PaidAmount > 0 {
		opID := middleware.GetAdminID(c)
		payment := models.PaymentRecord{
			ParkingLotID:  lotID,
			MonthlyCardID: &card.ID,
			PaymentType:   "monthly",
			Amount:        req.PaidAmount,
			PaymentMethod: req.PaymentMethod,
			TransactionNo: req.TransactionNo,
			OperatorID:    &opID,
			Status:        "success",
		}
		tx.Create(&payment)
	}
	tx.Commit()

	utils.OK(c, card)
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

	var records []models.ParkingRecord
	utils.DB.Where("monthly_card_id = ?", card.ID).Order("entry_time DESC").Limit(20).Find(&records)

	utils.OK(c, gin.H{
		"card":            card,
		"remaining_days":  int(time.Until(card.EndDate).Hours() / 24),
		"recent_records":  records,
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
	var newStart, newEnd time.Time
	if req.StartFromNow || card.EndDate.Before(time.Now()) {
		newStart = time.Now()
	} else {
		newStart = card.EndDate.AddDate(0, 0, 1)
	}
	newEnd = newStart.AddDate(0, req.Months, -1)

	card.PlanType = req.PlanType
	card.Price = req.Price
	card.StartDate = newStart
	card.EndDate = newEnd
	card.Status = "active"
	card.PaidAmount += req.PaidAmount
	if req.PaymentMethod != "" {
		card.PaymentMethod = req.PaymentMethod
	}

	tx := utils.DB.Begin()
	if err := tx.Save(&card).Error; err != nil {
		tx.Rollback()
		utils.InternalError(c, "续费失败")
		return
	}
	opID := middleware.GetAdminID(c)
	payment := models.PaymentRecord{
		ParkingLotID:  card.ParkingLotID,
		MonthlyCardID: &card.ID,
		PaymentType:   "monthly",
		Amount:        req.PaidAmount,
		PaymentMethod: req.PaymentMethod,
		TransactionNo: req.TransactionNo,
		OperatorID:    &opID,
		Status:        "success",
	}
	tx.Create(&payment)
	tx.Commit()

	utils.OK(c, card)
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
	var card models.MonthlyCard
	err := utils.DB.Where("parking_lot_id = ? AND vehicle_plate = ? AND status = ? AND start_date <= CURRENT_DATE AND end_date >= CURRENT_DATE",
		lotID, plate, "active").Order("end_date DESC").First(&card).Error
	if err != nil {
		utils.OK(c, gin.H{
			"has_card":       false,
			"is_active":      false,
			"remaining_days": 0,
		})
		return
	}
	utils.OK(c, gin.H{
		"has_card":       true,
		"is_active":      true,
		"card":           card,
		"remaining_days": int(time.Until(card.EndDate).Hours() / 24),
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
