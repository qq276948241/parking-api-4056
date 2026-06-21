package service

import (
	"errors"
	"parking-system/internal/models"
	"parking-system/internal/utils"
	"time"

	"github.com/google/uuid"
)

type MonthlyCardService struct{}

func NewMonthlyCardService() *MonthlyCardService {
	return &MonthlyCardService{}
}

var (
	ErrCardNotFound   = errors.New("未找到有效月卡")
	ErrCardExpired    = errors.New("月卡已过期")
	ErrCardSuspended  = errors.New("月卡已停用")
	ErrInvalidDateRange = errors.New("日期范围不合法")
)

type CardValidity struct {
	Valid          bool              `json:"valid"`
	Reason         string            `json:"reason,omitempty"`
	Card           *models.MonthlyCard `json:"card,omitempty"`
	RemainingDays  int               `json:"remaining_days"`
	RemainingHours int               `json:"remaining_hours"`
}

func (s *MonthlyCardService) CheckVehicleActiveCard(parkingLotID uuid.UUID, plate string) (*models.MonthlyCard, error) {
	var card models.MonthlyCard
	today := time.Now()
	err := utils.DB.Where(
		"parking_lot_id = ? AND vehicle_plate = ? AND status = ? AND start_date <= ? AND end_date >= ?",
		parkingLotID, plate, "active", today, today,
	).Order("end_date DESC").First(&card).Error
	if err != nil {
		return nil, ErrCardNotFound
	}
	return &card, nil
}

func (s *MonthlyCardService) GetValidity(card *models.MonthlyCard) CardValidity {
	if card == nil {
		return CardValidity{Valid: false, Reason: "月卡不存在"}
	}
	now := time.Now()
	switch card.Status {
	case "expired":
		return CardValidity{Valid: false, Reason: ErrCardExpired.Error(), Card: card}
	case "suspended":
		return CardValidity{Valid: false, Reason: ErrCardSuspended.Error(), Card: card}
	}
	if card.StartDate.After(now) {
		return CardValidity{Valid: false, Reason: "月卡尚未生效", Card: card}
	}
	if card.EndDate.Before(now) {
		return CardValidity{Valid: false, Reason: ErrCardExpired.Error(), Card: card}
	}
	remain := time.Until(card.EndDate)
	return CardValidity{
		Valid:          true,
		Card:           card,
		RemainingDays:  int(remain.Hours() / 24),
		RemainingHours: int(remain.Hours()),
	}
}

func (s *MonthlyCardService) ComputeEndDate(startDate time.Time, planType string, months int) (time.Time, error) {
	if months <= 0 {
		switch planType {
		case "monthly":
			months = 1
		case "quarterly":
			months = 3
		case "yearly":
			months = 12
		default:
			return time.Time{}, ErrInvalidDateRange
		}
	}
	end := startDate.AddDate(0, months, -1)
	if end.Before(startDate) {
		return time.Time{}, ErrInvalidDateRange
	}
	return end, nil
}

func (s *MonthlyCardService) Renew(card *models.MonthlyCard, planType string, months int, startFromNow bool) (time.Time, time.Time, error) {
	if months <= 0 {
		return time.Time{}, time.Time{}, ErrInvalidDateRange
	}
	var newStart time.Time
	now := time.Now()
	if startFromNow || card.EndDate.Before(now) {
		newStart = now
	} else {
		newStart = card.EndDate.AddDate(0, 0, 1)
	}
	newEnd, err := s.ComputeEndDate(newStart, planType, months)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return newStart, newEnd, nil
}

func (s *MonthlyCardService) AutoRefreshStatus(cardID uuid.UUID) error {
	var card models.MonthlyCard
	if err := utils.DB.First(&card, cardID).Error; err != nil {
		return err
	}
	if card.Status == "active" && card.EndDate.Before(time.Now()) {
		card.Status = "expired"
		return utils.DB.Save(&card).Error
	}
	return nil
}

func (s *MonthlyCardService) ListExpiring(parkingLotID *uuid.UUID, days int) ([]models.MonthlyCard, int64, error) {
	today := time.Now()
	target := today.AddDate(0, 0, days)
	db := utils.DB.Model(&models.MonthlyCard{}).
		Where("status = ? AND end_date <= ? AND end_date >= ?", "active", target, today)
	if parkingLotID != nil {
		db = db.Where("parking_lot_id = ?", *parkingLotID)
	}
	var total int64
	db.Count(&total)
	var list []models.MonthlyCard
	db.Order("end_date ASC").Find(&list)
	return list, total, nil
}
