package utils

import (
	"math"
	"parking-system/internal/models"
	"sort"
	"time"
)

type ParkingFeeCalc struct {
	HourlyRate  float64
	DailyMax    float64
	FreeMinutes int
	FeeTiers    models.FeeTiers
}

type FeeResult struct {
	DurationMin   int
	ChargeableMin int
	Hours         float64
	BaseAmount    float64
	FinalAmount   float64
	Breakdown     []TierBreakdown `json:",omitempty"`
}

type TierBreakdown struct {
	StartHour  float64 `json:"start_hour"`
	EndHour    float64 `json:"end_hour"`
	Hours      float64 `json:"hours"`
	Rate       float64 `json:"rate"`
	Amount     float64 `json:"amount"`
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
	hours := math.Ceil(float64(chargeable) / 60.0)

	var baseAmount float64
	var breakdown []TierBreakdown

	if chargeable > 0 {
		totalFullDays := int(duration / (24 * time.Hour))
		remainderDur := duration - time.Duration(totalFullDays)*24*time.Hour

		var remainderChargeableMin int
		if totalFullDays >= 1 {
			remainderChargeableMin = int(remainderDur.Minutes())
		} else {
			remainderChargeableMin = chargeable
		}

		if remainderChargeableMin < 0 {
			remainderChargeableMin = 0
		}

		remainderAmount, bd := calcAmountForMinutes(remainderChargeableMin, calc)
		breakdown = bd
		if calc.DailyMax > 0 && remainderAmount > calc.DailyMax {
			remainderAmount = calc.DailyMax
		}

		baseAmount = float64(totalFullDays)*calc.DailyMax + remainderAmount
	}

	return FeeResult{
		DurationMin:   durationMin,
		ChargeableMin: chargeable,
		Hours:         hours,
		BaseAmount:    round2(baseAmount),
		FinalAmount:   round2(baseAmount),
		Breakdown:     breakdown,
	}
}

func calcAmountForMinutes(minutes int, calc ParkingFeeCalc) (float64, []TierBreakdown) {
	if minutes <= 0 {
		return 0, nil
	}
	hours := math.Ceil(float64(minutes) / 60.0)

	tiers := normalizeTiers(calc.FeeTiers, calc.HourlyRate)
	if len(tiers) == 0 {
		return round2(hours * calc.HourlyRate), nil
	}

	var total float64
	remaining := hours
	var breakdown []TierBreakdown

	for _, t := range tiers {
		if remaining <= 0 {
			break
		}
		span := t.EndHour - t.StartHour
		if span <= 0 {
			continue
		}
		inTier := remaining
		if span < inTier {
			inTier = span
		}
		amount := inTier * t.Rate
		total += amount
		breakdown = append(breakdown, TierBreakdown{
			StartHour: t.StartHour,
			EndHour:   t.EndHour,
			Hours:     round2(inTier),
			Rate:      t.Rate,
			Amount:    round2(amount),
		})
		remaining -= inTier
	}

	return round2(total), breakdown
}

func normalizeTiers(tiers models.FeeTiers, fallbackRate float64) []models.FeeTier {
	if len(tiers) == 0 {
		return []models.FeeTier{{StartHour: 0, EndHour: 99999, Rate: fallbackRate}}
	}
	sorted := make([]models.FeeTier, len(tiers))
	copy(sorted, tiers)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StartHour < sorted[j].StartHour
	})
	result := make([]models.FeeTier, 0, len(sorted))
	for i, t := range sorted {
		rate := t.Rate
		if rate <= 0 {
			rate = fallbackRate
		}
		end := t.EndHour
		if end <= t.StartHour {
			if i+1 < len(sorted) {
				end = sorted[i+1].StartHour
			} else {
				end = t.StartHour + 1
			}
		}
		result = append(result, models.FeeTier{
			StartHour: t.StartHour,
			EndHour:   end,
			Rate:      rate,
		})
	}
	if len(result) > 0 {
		last := &result[len(result)-1]
		if last.EndHour < 99999 {
			last.EndHour = 99999
		}
	}
	return result
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
