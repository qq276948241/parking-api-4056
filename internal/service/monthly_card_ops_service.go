package service

import (
	"fmt"
	"parking-system/internal/models"
	"parking-system/internal/utils"
	"time"

	"github.com/google/uuid"
)

type MonthlyCardOpsService struct {
	CardSvc *MonthlyCardService
}

func NewMonthlyCardOpsService() *MonthlyCardOpsService {
	return &MonthlyCardOpsService{CardSvc: NewMonthlyCardService()}
}

type CreateCardParams struct {
	ParkingLotID  uuid.UUID
	CardNumber    string
	VehiclePlate  string
	OwnerName     string
	OwnerPhone    string
	PlanName      string
	PlanType      string
	Price         float64
	StartDate     time.Time
	Months        int
	EndDate       *time.Time
	PaidAmount    float64
	PaymentMethod string
	TransactionNo string
	OperatorID    uuid.UUID
	Remarks       string
}

type CreateCardResult struct {
	Card    *models.MonthlyCard
	Payment *models.PaymentRecord
}

func (s *MonthlyCardOpsService) CreateCard(p CreateCardParams) (*CreateCardResult, error) {
	startDate := p.StartDate
	if startDate.IsZero() {
		startDate = time.Now()
	}

	var endDate time.Time
	if p.EndDate != nil {
		endDate = *p.EndDate
		if endDate.Before(startDate) {
			return nil, ErrInvalidDateRange
		}
	} else {
		ed, err := s.CardSvc.ComputeEndDate(startDate, p.PlanType, p.Months)
		if err != nil {
			return nil, err
		}
		endDate = ed
	}

	cardNumber := p.CardNumber
	if cardNumber == "" {
		cardNumber = fmt.Sprintf("MC%s%s", p.ParkingLotID.String()[:8], time.Now().Format("20060102150405"))
	}

	status := "active"
	if endDate.Before(time.Now()) {
		status = "expired"
	}

	card := &models.MonthlyCard{
		ParkingLotID:  p.ParkingLotID,
		CardNumber:    cardNumber,
		VehiclePlate:  p.VehiclePlate,
		OwnerName:     p.OwnerName,
		OwnerPhone:    p.OwnerPhone,
		PlanName:      p.PlanName,
		PlanType:      p.PlanType,
		Price:         p.Price,
		StartDate:     startDate,
		EndDate:       endDate,
		Status:        status,
		PaidAmount:    p.PaidAmount,
		PaymentMethod: p.PaymentMethod,
		Remarks:       p.Remarks,
	}

	tx := utils.DB.Begin()
	if err := tx.Create(card).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	var payment *models.PaymentRecord
	if p.PaidAmount > 0 {
		payment = &models.PaymentRecord{
			ParkingLotID:  p.ParkingLotID,
			MonthlyCardID: &card.ID,
			PaymentType:   "monthly",
			Amount:        p.PaidAmount,
			PaymentMethod: p.PaymentMethod,
			TransactionNo: p.TransactionNo,
			OperatorID:    &p.OperatorID,
			Status:        "success",
		}
		if err := tx.Create(payment).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	tx.Commit()

	return &CreateCardResult{Card: card, Payment: payment}, nil
}

type RenewCardParams struct {
	CardID        uuid.UUID
	PlanType      string
	Months        int
	Price         float64
	PaidAmount    float64
	PaymentMethod string
	TransactionNo string
	OperatorID    uuid.UUID
	StartFromNow  bool
}

type RenewCardResult struct {
	Card       *models.MonthlyCard
	Payment    *models.PaymentRecord
	NewStart   time.Time
	NewEnd     time.Time
}

func (s *MonthlyCardOpsService) RenewCard(p RenewCardParams) (*RenewCardResult, error) {
	var card models.MonthlyCard
	if err := utils.DB.First(&card, p.CardID).Error; err != nil {
		return nil, ErrCardNotFound
	}
	newStart, newEnd, err := s.CardSvc.Renew(&card, p.PlanType, p.Months, p.StartFromNow)
	if err != nil {
		return nil, err
	}

	card.PlanType = p.PlanType
	card.Price = p.Price
	card.StartDate = newStart
	card.EndDate = newEnd
	card.Status = "active"
	card.PaidAmount += p.PaidAmount
	if p.PaymentMethod != "" {
		card.PaymentMethod = p.PaymentMethod
	}

	tx := utils.DB.Begin()
	if err := tx.Save(&card).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	var payment *models.PaymentRecord
	if p.PaidAmount > 0 {
		payment = &models.PaymentRecord{
			ParkingLotID:  card.ParkingLotID,
			MonthlyCardID: &card.ID,
			PaymentType:   "monthly",
			Amount:        p.PaidAmount,
			PaymentMethod: p.PaymentMethod,
			TransactionNo: p.TransactionNo,
			OperatorID:    &p.OperatorID,
			Status:        "success",
		}
		if err := tx.Create(payment).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	tx.Commit()

	return &RenewCardResult{
		Card:     &card,
		Payment:  payment,
		NewStart: newStart,
		NewEnd:   newEnd,
	}, nil
}

func (s *MonthlyCardOpsService) GetCardWithRecords(id uuid.UUID) (*models.MonthlyCard, []models.ParkingRecord, CardValidity, error) {
	var card models.MonthlyCard
	if err := utils.DB.First(&card, id).Error; err != nil {
		return nil, nil, CardValidity{}, ErrCardNotFound
	}
	var records []models.ParkingRecord
	utils.DB.Where("monthly_card_id = ?", card.ID).Order("entry_time DESC").Limit(20).Find(&records)
	validity := s.CardSvc.GetValidity(&card)
	return &card, records, validity, nil
}

func (s *MonthlyCardOpsService) RefreshStatus(id uuid.UUID) (*models.MonthlyCard, error) {
	var card models.MonthlyCard
	if err := utils.DB.First(&card, id).Error; err != nil {
		return nil, ErrCardNotFound
	}
	if card.Status == "active" && card.EndDate.Before(time.Now()) {
		card.Status = "expired"
		if err := utils.DB.Save(&card).Error; err != nil {
			return nil, err
		}
	}
	return &card, nil
}
