package handlers

import (
	"parking-system/internal/middleware"
	"parking-system/internal/models"
	"parking-system/internal/utils"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ParkingRecordHandler struct{}

func NewParkingRecordHandler() *ParkingRecordHandler {
	return &ParkingRecordHandler{}
}

type EntryReq struct {
	ParkingLotID string `json:"parking_lot_id"`
	SpaceID      string `json:"space_id"`
	VehiclePlate string `json:"vehicle_plate" binding:"required"`
	VehicleType  string `json:"vehicle_type" binding:"omitempty,oneof=car suv truck motorcycle"`
	EntryTime    string `json:"entry_time"`
}

type ExitReq struct {
	ExitTime       string  `json:"exit_time"`
	PaymentMethod  string  `json:"payment_method" binding:"omitempty,oneof=cash wechat alipay card monthly"`
	Discount       float64 `json:"discount"`
	PaidAmount     float64 `json:"paid_amount"`
	PaymentStatus  string  `json:"payment_status" binding:"omitempty,oneof=unpaid paid partial waived"`
	TransactionNo  string  `json:"transaction_no"`
	Remarks        string  `json:"remarks"`
}

type CalcFeeReq struct {
	RecordID     string `json:"record_id"`
	VehiclePlate string `json:"vehicle_plate"`
	ExitTime     string `json:"exit_time"`
}

func (h *ParkingRecordHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	plate := c.Query("vehicle_plate")
	status := c.Query("status")
	payStatus := c.Query("payment_status")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	isMonthly := c.Query("is_monthly")

	db := utils.DB.Model(&models.ParkingRecord{})
	db = scopedLotID(c, db, "parking_lot_id")

	if plate != "" {
		db = db.Where("vehicle_plate LIKE ?", "%"+plate+"%")
	}
	if status != "" {
		db = db.Where("status = ?", status)
	}
	if payStatus != "" {
		db = db.Where("payment_status = ?", payStatus)
	}
	if startDate != "" {
		db = db.Where("entry_time >= ?", startDate)
	}
	if endDate != "" {
		db = db.Where("entry_time <= ?", endDate+" 23:59:59")
	}
	if isMonthly != "" {
		monthly, _ := strconv.ParseBool(isMonthly)
		db = db.Where("is_monthly = ?", monthly)
	}

	var total int64
	db.Count(&total)
	var list []models.ParkingRecord
	db.Order("entry_time DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list)
	utils.OKPaged(c, list, total, page, pageSize)
}

func (h *ParkingRecordHandler) CurrentParking(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	plate := c.Query("vehicle_plate")

	db := utils.DB.Model(&models.ParkingRecord{}).Where("status = ?", "parking")
	db = scopedLotID(c, db, "parking_lot_id")

	if plate != "" {
		db = db.Where("vehicle_plate LIKE ?", "%"+plate+"%")
	}
	var total int64
	db.Count(&total)
	var list []models.ParkingRecord
	db.Order("entry_time ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list)
	utils.OKPaged(c, list, total, page, pageSize)
}

func (h *ParkingRecordHandler) Entry(c *gin.Context) {
	lotID, ok := getTargetLotID(c)
	if !ok {
		return
	}
	var req EntryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	req.ParkingLotID = lotID.String()

	var existing models.ParkingRecord
	utils.DB.Where("parking_lot_id = ? AND vehicle_plate = ? AND status = ?", lotID, req.VehiclePlate, "parking").
		First(&existing)
	if existing.ID != uuid.Nil {
		utils.BadRequest(c, "该车辆已在场内")
		return
	}

	var lot models.ParkingLot
	utils.DB.First(&lot, lotID)

	var card models.MonthlyCard
	var isMonthly bool
	utils.DB.Where("parking_lot_id = ? AND vehicle_plate = ? AND status = ? AND start_date <= CURRENT_DATE AND end_date >= CURRENT_DATE",
		lotID, req.VehiclePlate, "active").Order("end_date DESC").First(&card)
	if card.ID != uuid.Nil {
		isMonthly = true
	}

	entryTime := time.Now()
	if req.EntryTime != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", req.EntryTime, time.Local); err == nil {
			entryTime = t
		}
	}

	record := models.ParkingRecord{
		ParkingLotID: lotID,
		VehiclePlate: req.VehiclePlate,
		VehicleType:  "car",
		EntryTime:    entryTime,
		HourlyRate:   lot.HourlyRate,
		Status:       "parking",
		IsMonthly:    isMonthly,
	}
	if req.VehicleType != "" {
		record.VehicleType = req.VehicleType
	}
	if req.SpaceID != "" {
		if sid, err := uuid.Parse(req.SpaceID); err == nil {
			record.SpaceID = &sid
			utils.DB.Model(&models.ParkingSpace{}).Where("id = ?", sid).
				Updates(map[string]interface{}{"status": "occupied", "vehicle_plate": req.VehiclePlate})
		}
	}
	if isMonthly {
		record.MonthlyCardID = &card.ID
		record.PaymentStatus = "paid"
		record.PaymentMethod = "monthly"
	}

	if err := utils.DB.Create(&record).Error; err != nil {
		utils.InternalError(c, "入场登记失败")
		return
	}
	utils.OK(c, record)
}

func (h *ParkingRecordHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	var record models.ParkingRecord
	if err := utils.DB.First(&record, id).Error; err != nil {
		utils.NotFound(c, "记录不存在")
		return
	}
	if err := checkLotAccess(c, record.ParkingLotID); err != nil {
		utils.Forbidden(c, err.Error())
		return
	}
	utils.OK(c, record)
}

func (h *ParkingRecordHandler) CalcFee(c *gin.Context) {
	var record models.ParkingRecord
	if idStr := c.Query("record_id"); idStr != "" {
		id, err := uuid.Parse(idStr)
		if err != nil {
			utils.BadRequest(c, "记录ID格式错误")
			return
		}
		if err := utils.DB.First(&record, id).Error; err != nil {
			utils.NotFound(c, "记录不存在")
			return
		}
		if err := checkLotAccess(c, record.ParkingLotID); err != nil {
			utils.Forbidden(c, err.Error())
			return
		}
	} else {
		plate := c.Query("vehicle_plate")
		lotID, ok := getTargetLotID(c)
		if !ok {
			return
		}
		utils.DB.Where("parking_lot_id = ? AND vehicle_plate = ? AND status = ?", lotID, plate, "parking").
			Order("entry_time DESC").First(&record)
		if record.ID == uuid.Nil {
			utils.NotFound(c, "未找到该车辆在场记录")
			return
		}
	}

	exitTime := time.Now()
	if t := c.Query("exit_time"); t != "" {
		if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", t, time.Local); err == nil {
			exitTime = parsed
		}
	}

	var lot models.ParkingLot
	utils.DB.First(&lot, record.ParkingLotID)

	fee := utils.CalcParkingFee(record.EntryTime, exitTime, utils.ParkingFeeCalc{
		HourlyRate:  lot.HourlyRate,
		DailyMax:    lot.DailyMax,
		FreeMinutes: lot.FreeMinutes,
	})

	utils.OK(c, gin.H{
		"record_id":       record.ID,
		"vehicle_plate":   record.VehiclePlate,
		"entry_time":      record.EntryTime,
		"exit_time":       exitTime,
		"duration_min":    fee.DurationMin,
		"chargeable_min":  fee.ChargeableMin,
		"hours":           fee.Hours,
		"hourly_rate":     lot.HourlyRate,
		"daily_max":       lot.DailyMax,
		"free_minutes":    lot.FreeMinutes,
		"base_amount":     fee.BaseAmount,
		"final_amount":    fee.FinalAmount,
		"is_monthly":      record.IsMonthly,
		"monthly_card_id": record.MonthlyCardID,
	})
}

func (h *ParkingRecordHandler) Exit(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	var record models.ParkingRecord
	if err := utils.DB.First(&record, id).Error; err != nil {
		utils.NotFound(c, "记录不存在")
		return
	}
	if err := checkLotAccess(c, record.ParkingLotID); err != nil {
		utils.Forbidden(c, err.Error())
		return
	}
	if record.Status == "completed" {
		utils.BadRequest(c, "该记录已出场")
		return
	}
	var req ExitReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	exitTime := time.Now()
	if req.ExitTime != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", req.ExitTime, time.Local); err == nil {
			exitTime = t
		}
	}

	var lot models.ParkingLot
	utils.DB.First(&lot, record.ParkingLotID)

	fee := utils.CalcParkingFee(record.EntryTime, exitTime, utils.ParkingFeeCalc{
		HourlyRate:  lot.HourlyRate,
		DailyMax:    lot.DailyMax,
		FreeMinutes: lot.FreeMinutes,
	})

	totalAmount := fee.FinalAmount
	if record.IsMonthly {
		totalAmount = 0
	}
	discount := req.Discount
	if discount < 0 {
		discount = 0
	}
	finalAmount := totalAmount - discount
	if finalAmount < 0 {
		finalAmount = 0
	}

	paidAmount := req.PaidAmount
	payStatus := req.PaymentStatus
	if payStatus == "" {
		if paidAmount >= finalAmount {
			payStatus = "paid"
		} else if paidAmount > 0 {
			payStatus = "partial"
		} else {
			payStatus = "unpaid"
		}
	}
	if record.IsMonthly {
		payStatus = "paid"
		paidAmount = 0
	}

	paymentMethod := req.PaymentMethod
	if record.IsMonthly {
		paymentMethod = "monthly"
	}

	record.ExitTime = &exitTime
	record.DurationMin = fee.DurationMin
	record.TotalAmount = totalAmount
	record.Discount = discount
	record.PaidAmount = paidAmount
	record.PaymentStatus = payStatus
	record.PaymentMethod = paymentMethod
	record.Status = "completed"
	record.Remarks = req.Remarks

	tx := utils.DB.Begin()
	if err := tx.Save(&record).Error; err != nil {
		tx.Rollback()
		utils.InternalError(c, "出场登记失败")
		return
	}

	if record.SpaceID != nil {
		tx.Model(&models.ParkingSpace{}).Where("id = ?", *record.SpaceID).
			Updates(map[string]interface{}{"status": "available", "vehicle_plate": ""})
	}

	if paidAmount > 0 {
		opID := middleware.GetAdminID(c)
		payment := models.PaymentRecord{
			ParkingLotID:    record.ParkingLotID,
			ParkingRecordID: &record.ID,
			PaymentType:     "parking",
			Amount:          paidAmount,
			PaymentMethod:   paymentMethod,
			TransactionNo:   req.TransactionNo,
			OperatorID:      &opID,
			Status:          "success",
		}
		tx.Create(&payment)
	}

	tx.Commit()

	utils.OK(c, gin.H{
		"record":          record,
		"duration_min":    fee.DurationMin,
		"chargeable_min":  fee.ChargeableMin,
		"base_amount":     fee.BaseAmount,
		"total_amount":    totalAmount,
		"discount":        discount,
		"final_amount":    finalAmount,
		"paid_amount":     paidAmount,
	})
}

func (h *ParkingRecordHandler) Pay(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	var record models.ParkingRecord
	if err := utils.DB.First(&record, id).Error; err != nil {
		utils.NotFound(c, "记录不存在")
		return
	}
	if err := checkLotAccess(c, record.ParkingLotID); err != nil {
		utils.Forbidden(c, err.Error())
		return
	}
	if record.Status != "completed" {
		utils.BadRequest(c, "车辆尚未出场，不能单独缴费")
		return
	}
	type PayReq struct {
		Amount        float64 `json:"amount" binding:"required,min=0"`
		PaymentMethod string  `json:"payment_method" binding:"required,oneof=cash wechat alipay card"`
		TransactionNo string  `json:"transaction_no"`
	}
	var req PayReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	remain := record.TotalAmount - record.Discount - record.PaidAmount
	if remain <= 0 {
		utils.BadRequest(c, "该记录已无待缴费用")
		return
	}
	paid := req.Amount
	if paid > remain {
		paid = remain
	}

	tx := utils.DB.Begin()
	record.PaidAmount += paid
	if record.PaidAmount >= (record.TotalAmount - record.Discount) {
		record.PaymentStatus = "paid"
		record.PaymentMethod = req.PaymentMethod
	} else {
		record.PaymentStatus = "partial"
	}
	tx.Save(&record)

	opID := middleware.GetAdminID(c)
	payment := models.PaymentRecord{
		ParkingLotID:    record.ParkingLotID,
		ParkingRecordID: &record.ID,
		PaymentType:     "parking",
		Amount:          paid,
		PaymentMethod:   req.PaymentMethod,
		TransactionNo:   req.TransactionNo,
		OperatorID:      &opID,
		Status:          "success",
	}
	tx.Create(&payment)
	tx.Commit()

	utils.OK(c, record)
}

func (h *ParkingRecordHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	var record models.ParkingRecord
	if err := utils.DB.First(&record, id).Error; err != nil {
		utils.NotFound(c, "记录不存在")
		return
	}
	if err := checkLotAccess(c, record.ParkingLotID); err != nil {
		utils.Forbidden(c, err.Error())
		return
	}
	type UpdateReq struct {
		VehiclePlate  string     `json:"vehicle_plate"`
		VehicleType   string     `json:"vehicle_type" binding:"omitempty,oneof=car suv truck motorcycle"`
		EntryTime     *time.Time `json:"entry_time"`
		ExitTime      *time.Time `json:"exit_time"`
		Remarks       string     `json:"remarks"`
		Discount      *float64   `json:"discount"`
		PaymentStatus string     `json:"payment_status" binding:"omitempty,oneof=unpaid paid partial waived"`
	}
	var req UpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if req.VehiclePlate != "" {
		record.VehiclePlate = req.VehiclePlate
	}
	if req.VehicleType != "" {
		record.VehicleType = req.VehicleType
	}
	if req.EntryTime != nil {
		record.EntryTime = *req.EntryTime
	}
	if req.ExitTime != nil {
		record.ExitTime = req.ExitTime
	}
	if req.Remarks != "" {
		record.Remarks = req.Remarks
	}
	if req.Discount != nil {
		record.Discount = *req.Discount
	}
	if req.PaymentStatus != "" {
		record.PaymentStatus = req.PaymentStatus
	}
	if err := utils.DB.Save(&record).Error; err != nil {
		utils.InternalError(c, "更新记录失败")
		return
	}
	utils.OK(c, record)
}
