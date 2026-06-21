package service

import (
	"errors"
	"parking-system/internal/models"
	"parking-system/internal/utils"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ParkingService struct {
	FeeService        *FeeService
	MonthlyCardSvc    *MonthlyCardService
}

func NewParkingService() *ParkingService {
	return &ParkingService{
		FeeService:     NewFeeService(),
		MonthlyCardSvc: NewMonthlyCardService(),
	}
}

var (
	ErrLotNotFound     = errors.New("停车场不存在")
	ErrAlreadyParking  = errors.New("该车辆已在场内")
	ErrRecordNotFound  = errors.New("停车记录不存在")
	ErrAlreadyExited   = errors.New("该记录已出场")
	ErrNotExited       = errors.New("车辆尚未出场")
	ErrNoUnpaid        = errors.New("该记录已无待缴费用")
)

type EntryParams struct {
	ParkingLotID uuid.UUID
	SpaceID      *uuid.UUID
	VehiclePlate string
	VehicleType  string
	EntryTime    time.Time
}

type EntryResult struct {
	Record    *models.ParkingRecord
	IsMonthly bool
	Card      *models.MonthlyCard
}

func (s *ParkingService) VehicleEntry(p EntryParams) (*EntryResult, error) {
	var lot models.ParkingLot
	if err := utils.DB.First(&lot, p.ParkingLotID).Error; err != nil {
		return nil, ErrLotNotFound
	}

	var existing models.ParkingRecord
	err := utils.DB.Where(
		"parking_lot_id = ? AND vehicle_plate = ? AND status = ?",
		p.ParkingLotID, p.VehiclePlate, "parking",
	).First(&existing).Error
	if err == nil && existing.ID != uuid.Nil {
		return nil, ErrAlreadyParking
	}

	card, _ := s.MonthlyCardSvc.CheckVehicleActiveCard(p.ParkingLotID, p.VehiclePlate)
	isMonthly := card != nil

	entryTime := p.EntryTime
	if entryTime.IsZero() {
		entryTime = time.Now()
	}
	vehicleType := p.VehicleType
	if vehicleType == "" {
		vehicleType = "car"
	}

	record := &models.ParkingRecord{
		ParkingLotID: p.ParkingLotID,
		SpaceID:      p.SpaceID,
		VehiclePlate: p.VehiclePlate,
		VehicleType:  vehicleType,
		EntryTime:    entryTime,
		HourlyRate:   lot.HourlyRate,
		Status:       "parking",
		IsMonthly:    isMonthly,
	}
	if isMonthly {
		record.MonthlyCardID = &card.ID
		record.PaymentStatus = "paid"
		record.PaymentMethod = "monthly"
	}

	tx := utils.DB.Begin()
	if err := tx.Create(record).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if p.SpaceID != nil {
		tx.Model(&models.ParkingSpace{}).Where("id = ?", *p.SpaceID).
			Updates(map[string]interface{}{"status": "occupied", "vehicle_plate": p.VehiclePlate})
	}
	tx.Commit()

	return &EntryResult{Record: record, IsMonthly: isMonthly, Card: card}, nil
}

type CalcFeeParams struct {
	RecordID   *uuid.UUID
	Plate      string
	LotID      *uuid.UUID
	ExitTime   time.Time
}

type PreviewFeeResult struct {
	Record      *models.ParkingRecord
	ExitTime    time.Time
	Fee         CalcFeeResult
	IsMonthly   bool
	TotalAmount float64
}

func (s *ParkingService) PreviewFee(p CalcFeeParams) (*PreviewFeeResult, error) {
	var record models.ParkingRecord
	if p.RecordID != nil {
		if err := utils.DB.First(&record, *p.RecordID).Error; err != nil {
			return nil, ErrRecordNotFound
		}
	} else {
		if p.LotID == nil || p.Plate == "" {
			return nil, ErrRecordNotFound
		}
		err := utils.DB.Where(
			"parking_lot_id = ? AND vehicle_plate = ? AND status = ?",
			*p.LotID, p.Plate, "parking",
		).Order("entry_time DESC").First(&record).Error
		if err != nil {
			return nil, ErrRecordNotFound
		}
	}

	exitTime := p.ExitTime
	if exitTime.IsZero() {
		exitTime = time.Now()
	}

	var lot models.ParkingLot
	utils.DB.First(&lot, record.ParkingLotID)
	cfg := s.FeeService.BuildConfig(&lot)
	fee := s.FeeService.CalcFee(record.EntryTime, exitTime, cfg)

	totalAmount := fee.FinalAmount
	if record.IsMonthly {
		totalAmount = 0
	}

	return &PreviewFeeResult{
		Record:      &record,
		ExitTime:    exitTime,
		Fee:         fee,
		IsMonthly:   record.IsMonthly,
		TotalAmount: totalAmount,
	}, nil
}

type ExitParams struct {
	RecordID      uuid.UUID
	ExitTime      time.Time
	PaymentMethod string
	Discount      float64
	PaidAmount    float64
	PaymentStatus string
	TransactionNo string
	OperatorID    uuid.UUID
	Remarks       string
}

type ExitResult struct {
	Record        *models.ParkingRecord
	Fee           CalcFeeResult
	TotalAmount   float64
	Discount      float64
	FinalAmount   float64
	PaidAmount    float64
	PaymentStatus string
}

func (s *ParkingService) VehicleExit(p ExitParams) (*ExitResult, error) {
	var record models.ParkingRecord
	if err := utils.DB.First(&record, p.RecordID).Error; err != nil {
		return nil, ErrRecordNotFound
	}
	if record.Status == "completed" {
		return nil, ErrAlreadyExited
	}

	exitTime := p.ExitTime
	if exitTime.IsZero() {
		exitTime = time.Now()
	}

	var lot models.ParkingLot
	utils.DB.First(&lot, record.ParkingLotID)
	cfg := s.FeeService.BuildConfig(&lot)
	fee := s.FeeService.CalcFee(record.EntryTime, exitTime, cfg)

	totalAmount := fee.FinalAmount
	if record.IsMonthly {
		totalAmount = 0
	}
	discount := p.Discount
	if discount < 0 {
		discount = 0
	}
	finalAmount := totalAmount - discount
	if finalAmount < 0 {
		finalAmount = 0
	}

	paidAmount := p.PaidAmount
	payStatus := p.PaymentStatus
	if payStatus == "" {
		if paidAmount >= finalAmount {
			payStatus = "paid"
		} else if paidAmount > 0 {
			payStatus = "partial"
		} else {
			payStatus = "unpaid"
		}
	}

	paymentMethod := p.PaymentMethod
	if record.IsMonthly {
		payStatus = "paid"
		paidAmount = 0
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
	record.Remarks = p.Remarks

	tx := utils.DB.Begin()
	if err := tx.Save(&record).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if record.SpaceID != nil {
		tx.Model(&models.ParkingSpace{}).Where("id = ?", *record.SpaceID).
			Updates(map[string]interface{}{"status": "available", "vehicle_plate": ""})
	}
	if paidAmount > 0 {
		payment := models.PaymentRecord{
			ParkingLotID:    record.ParkingLotID,
			ParkingRecordID: &record.ID,
			PaymentType:     "parking",
			Amount:          paidAmount,
			PaymentMethod:   paymentMethod,
			TransactionNo:   p.TransactionNo,
			OperatorID:      &p.OperatorID,
			Status:          "success",
		}
		tx.Create(&payment)
	}
	tx.Commit()

	return &ExitResult{
		Record:        &record,
		Fee:           fee,
		TotalAmount:   totalAmount,
		Discount:      discount,
		FinalAmount:   finalAmount,
		PaidAmount:    paidAmount,
		PaymentStatus: payStatus,
	}, nil
}

type PayParams struct {
	RecordID      uuid.UUID
	Amount        float64
	PaymentMethod string
	TransactionNo string
	OperatorID    uuid.UUID
}

type PayResult struct {
	Record      *models.ParkingRecord
	PaidNow     float64
	Remaining   float64
	PaymentRecord *models.PaymentRecord
}

func (s *ParkingService) PayRecord(p PayParams) (*PayResult, error) {
	var record models.ParkingRecord
	if err := utils.DB.First(&record, p.RecordID).Error; err != nil {
		return nil, ErrRecordNotFound
	}
	if record.Status != "completed" {
		return nil, ErrNotExited
	}

	remain := record.TotalAmount - record.Discount - record.PaidAmount
	if remain <= 0 {
		return nil, ErrNoUnpaid
	}
	paid := p.Amount
	if paid > remain {
		paid = remain
	}

	tx := utils.DB.Begin()
	record.PaidAmount += paid
	if record.PaidAmount >= (record.TotalAmount - record.Discount) {
		record.PaymentStatus = "paid"
		record.PaymentMethod = p.PaymentMethod
	} else {
		record.PaymentStatus = "partial"
	}
	if err := tx.Save(&record).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	payment := &models.PaymentRecord{
		ParkingLotID:    record.ParkingLotID,
		ParkingRecordID: &record.ID,
		PaymentType:     "parking",
		Amount:          paid,
		PaymentMethod:   p.PaymentMethod,
		TransactionNo:   p.TransactionNo,
		OperatorID:      &p.OperatorID,
		Status:          "success",
	}
	if err := tx.Create(payment).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	tx.Commit()

	return &PayResult{
		Record:        &record,
		PaidNow:       paid,
		Remaining:     (record.TotalAmount - record.Discount) - record.PaidAmount,
		PaymentRecord: payment,
	}, nil
}

func (s *ParkingService) GetRecord(id uuid.UUID) (*models.ParkingRecord, error) {
	var r models.ParkingRecord
	if err := utils.DB.First(&r, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}
	return &r, nil
}
