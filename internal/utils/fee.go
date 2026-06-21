package utils

import (
	"math"
	"time"
)

type ParkingFeeCalc struct {
	HourlyRate  float64
	DailyMax    float64
	FreeMinutes int
}

type FeeResult struct {
	DurationMin   int
	ChargeableMin int
	Hours         float64
	BaseAmount    float64
	FinalAmount   float64
}

func CalcParkingFee(entryTime, exitTime time.Time, calc ParkingFeeCalc) FeeResult {
	duration := exitTime.Sub(entryTime)
	durationMin := int(duration.Minutes())
	if durationMin < 0 {
		durationMin = 0
	}
	chargeable := durationMin - calc.FreeMinutes
	if chargeable < 0 {
		chargeable = 0
	}

	var baseAmount float64
	if chargeable > 0 {
		totalDays := duration / (24 * time.Hour)
		remainder := duration % (24 * time.Hour)

		remainderMin := int(remainder.Minutes())
		remainderChargeable := 0
		if totalDays >= 1 {
			remainderChargeable = remainderMin
		} else {
			remainderChargeable = chargeable
		}

		hours := math.Ceil(float64(remainderChargeable) / 60.0)
		remainderAmount := hours * calc.HourlyRate
		if calc.DailyMax > 0 && remainderAmount > calc.DailyMax {
			remainderAmount = calc.DailyMax
		}

		baseAmount = float64(totalDays)*calc.DailyMax + remainderAmount
	}

	hours := math.Ceil(float64(chargeable) / 60.0)
	return FeeResult{
		DurationMin:   durationMin,
		ChargeableMin: chargeable,
		Hours:         hours,
		BaseAmount:    round2(baseAmount),
		FinalAmount:   round2(baseAmount),
	}
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
