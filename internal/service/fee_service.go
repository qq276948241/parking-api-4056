package service

import (
	"math"
	"parking-system/internal/models"
	"parking-system/internal/utils"
	"time"
)

type FeeService struct{}

func NewFeeService() *FeeService {
	return &FeeService{}
}

type CalcFeeResult struct {
	DurationMin   int                      `json:"duration_min"`
	ChargeableMin int                      `json:"chargeable_min"`
	Hours         float64                  `json:"hours"`
	BaseAmount    float64                  `json:"base_amount"`
	FinalAmount   float64                  `json:"final_amount"`
	Breakdown     []utils.TierBreakdown    `json:"breakdown,omitempty"`
	Config        FeeCalcConfig            `json:"config"`
}

type FeeCalcConfig struct {
	HourlyRate  float64          `json:"hourly_rate"`
	DailyMax    float64          `json:"daily_max"`
	FreeMinutes int              `json:"free_minutes"`
	FeeTiers    models.FeeTiers  `json:"fee_tiers"`
}

func (s *FeeService) BuildConfig(lot *models.ParkingLot) FeeCalcConfig {
	if lot == nil {
		return FeeCalcConfig{HourlyRate: 5, DailyMax: 50, FreeMinutes: 30}
	}
	return FeeCalcConfig{
		HourlyRate:  lot.HourlyRate,
		DailyMax:    lot.DailyMax,
		FreeMinutes: lot.FreeMinutes,
		FeeTiers:    lot.FeeTiers,
	}
}

func (s *FeeService) CalcFee(entryTime, exitTime time.Time, cfg FeeCalcConfig) CalcFeeResult {
	raw := utils.CalcParkingFee(entryTime, exitTime, utils.ParkingFeeCalc{
		HourlyRate:  cfg.HourlyRate,
		DailyMax:    cfg.DailyMax,
		FreeMinutes: cfg.FreeMinutes,
		FeeTiers:    cfg.FeeTiers,
	})
	return CalcFeeResult{
		DurationMin:   raw.DurationMin,
		ChargeableMin: raw.ChargeableMin,
		Hours:         raw.Hours,
		BaseAmount:    raw.BaseAmount,
		FinalAmount:   raw.FinalAmount,
		Breakdown:     raw.Breakdown,
		Config:        cfg,
	}
}

func (s *FeeService) ApplyDiscount(fee CalcFeeResult, discount float64) (base, after float64) {
	base = fee.FinalAmount
	if discount < 0 {
		discount = 0
	}
	after = base - discount
	if after < 0 {
		after = 0
	}
	return base, round2(after)
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
