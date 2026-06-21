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

type ParkingRecordHandler struct {
	svc *service.ParkingService
}

func NewParkingRecordHandler() *ParkingRecordHandler {
	return &ParkingRecordHandler{svc: service.NewParkingService()}
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

	entryTime := time.Now()
	if req.EntryTime != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", req.EntryTime, time.Local); err == nil {
			entryTime = t
		}
	}
	var spaceID *uuid.UUID
	if req.SpaceID != "" {
		if sid, err := uuid.Parse(req.SpaceID); err == nil {
			spaceID = &sid
		}
	}

	res, err := h.svc.VehicleEntry(service.EntryParams{
		ParkingLotID: lotID,
		SpaceID:      spaceID,
		VehiclePlate: req.VehiclePlate,
		VehicleType:  req.VehicleType,
		EntryTime:    entryTime,
	})
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.OK(c, res.Record)
}

func (h *ParkingRecordHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	record, err := h.svc.GetRecord(id)
	if err != nil {
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
	var recordID *uuid.UUID
	if idStr := c.Query("record_id"); idStr != "" {
		id, err := uuid.Parse(idStr)
		if err != nil {
			utils.BadRequest(c, "记录ID格式错误")
			return
		}
		rec, err := h.svc.GetRecord(id)
		if err != nil {
			utils.NotFound(c, "记录不存在")
			return
		}
		if err := checkLotAccess(c, rec.ParkingLotID); err != nil {
			utils.Forbidden(c, err.Error())
			return
		}
		recordID = &id
	}

	exitTime := time.Now()
	if t := c.Query("exit_time"); t != "" {
		if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", t, time.Local); err == nil {
			exitTime = parsed
		}
	}

	var lotID *uuid.UUID
	var plate string
	if recordID == nil {
		plate = c.Query("vehicle_plate")
		lid, ok := getTargetLotID(c)
		if !ok {
			return
		}
		lotID = &lid
	}

	preview, err := h.svc.PreviewFee(service.CalcFeeParams{
		RecordID: recordID,
		Plate:    plate,
		LotID:    lotID,
		ExitTime: exitTime,
	})
	if err != nil {
		utils.NotFound(c, err.Error())
		return
	}
	if err := checkLotAccess(c, preview.Record.ParkingLotID); err != nil {
		utils.Forbidden(c, err.Error())
		return
	}

	utils.OK(c, gin.H{
		"record_id":       preview.Record.ID,
		"vehicle_plate":   preview.Record.VehiclePlate,
		"entry_time":      preview.Record.EntryTime,
		"exit_time":       preview.ExitTime,
		"duration_min":    preview.Fee.DurationMin,
		"chargeable_min":  preview.Fee.ChargeableMin,
		"hours":           preview.Fee.Hours,
		"hourly_rate":     preview.Fee.Config.HourlyRate,
		"daily_max":       preview.Fee.Config.DailyMax,
		"free_minutes":    preview.Fee.Config.FreeMinutes,
		"fee_tiers":       preview.Fee.Config.FeeTiers,
		"breakdown":       preview.Fee.Breakdown,
		"base_amount":     preview.Fee.BaseAmount,
		"final_amount":    preview.TotalAmount,
		"is_monthly":      preview.IsMonthly,
		"monthly_card_id": preview.Record.MonthlyCardID,
	})
}

func (h *ParkingRecordHandler) Exit(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	rec, err := h.svc.GetRecord(id)
	if err != nil {
		utils.NotFound(c, "记录不存在")
		return
	}
	if err := checkLotAccess(c, rec.ParkingLotID); err != nil {
		utils.Forbidden(c, err.Error())
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

	res, err := h.svc.VehicleExit(service.ExitParams{
		RecordID:      id,
		ExitTime:      exitTime,
		PaymentMethod: req.PaymentMethod,
		Discount:      req.Discount,
		PaidAmount:    req.PaidAmount,
		PaymentStatus: req.PaymentStatus,
		TransactionNo: req.TransactionNo,
		OperatorID:    middleware.GetAdminID(c),
		Remarks:       req.Remarks,
	})
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}

	utils.OK(c, gin.H{
		"record":          res.Record,
		"duration_min":    res.Fee.DurationMin,
		"chargeable_min":  res.Fee.ChargeableMin,
		"breakdown":       res.Fee.Breakdown,
		"base_amount":     res.Fee.BaseAmount,
		"total_amount":    res.TotalAmount,
		"discount":        res.Discount,
		"final_amount":    res.FinalAmount,
		"paid_amount":     res.PaidAmount,
	})
}

func (h *ParkingRecordHandler) Pay(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	rec, err := h.svc.GetRecord(id)
	if err != nil {
		utils.NotFound(c, "记录不存在")
		return
	}
	if err := checkLotAccess(c, rec.ParkingLotID); err != nil {
		utils.Forbidden(c, err.Error())
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

	res, err := h.svc.PayRecord(service.PayParams{
		RecordID:      id,
		Amount:        req.Amount,
		PaymentMethod: req.PaymentMethod,
		TransactionNo: req.TransactionNo,
		OperatorID:    middleware.GetAdminID(c),
	})
	if err != nil {
		utils.BadRequest(c, err.Error())
		return
	}
	utils.OK(c, res.Record)
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
