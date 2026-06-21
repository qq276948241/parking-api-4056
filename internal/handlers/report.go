package handlers

import (
	"parking-system/internal/middleware"
	"parking-system/internal/models"
	"parking-system/internal/utils"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ReportHandler struct{}

func NewReportHandler() *ReportHandler {
	return &ReportHandler{}
}

type OverviewResp struct {
	TotalLots          int     `json:"total_lots"`
	TotalSpaces        int     `json:"total_spaces"`
	AvailableSpaces    int     `json:"available_spaces"`
	OccupiedSpaces     int     `json:"occupied_spaces"`
	CurrentParking     int64   `json:"current_parking"`
	ActiveMonthlyCards int64   `json:"active_monthly_cards"`
	TodayEntry         int64   `json:"today_entry"`
	TodayExit          int64   `json:"today_exit"`
	TodayParkingIncome float64 `json:"today_parking_income"`
	TodayMonthlyIncome float64 `json:"today_monthly_income"`
	TodayTotalIncome   float64 `json:"today_total_income"`
	MonthEntry         int64   `json:"month_entry"`
	MonthExit          int64   `json:"month_exit"`
	MonthParkingIncome float64 `json:"month_parking_income"`
	MonthMonthlyIncome float64 `json:"month_monthly_income"`
	MonthTotalIncome   float64 `json:"month_total_income"`
}

func sumAmount(db *gorm.DB, ptype, cond string) float64 {
	type row struct{ S float64 }
	var r row
	db.Select("COALESCE(SUM(amount),0) as s").
		Where("payment_type = ? AND status = ? AND "+cond, ptype, "success").
		Scan(&r)
	return r.S
}

func (h *ReportHandler) Overview(c *gin.Context) {
	db := utils.DB
	var resp OverviewResp

	lotsDB := db.Model(&models.ParkingLot{})
	if !middleware.IsSuperAdmin(c) {
		pid := middleware.GetParkingLotID(c)
		if pid != nil {
			lotsDB = lotsDB.Where("id = ?", *pid)
		} else {
			lotsDB = lotsDB.Where("1=0")
		}
	}
	var lots []models.ParkingLot
	lotsDB.Find(&lots)
	resp.TotalLots = len(lots)
	for _, l := range lots {
		resp.TotalSpaces += l.TotalSpaces
	}

	spacesDB := db.Model(&models.ParkingSpace{})
	spacesDB = scopedLotID(c, spacesDB, "parking_lot_id")
	var avail int64
	spacesDB.Session(&gorm.Session{}).Where("status = ?", "available").Count(&avail)
	resp.AvailableSpaces = int(avail)
	var occupied int64
	spacesDB.Session(&gorm.Session{}).Where("status = ?", "occupied").Count(&occupied)
	resp.OccupiedSpaces = int(occupied)

	recordsDB := db.Model(&models.ParkingRecord{})
	recordsDB = scopedLotID(c, recordsDB, "parking_lot_id")
	recordsDB.Session(&gorm.Session{}).Where("status = ?", "parking").Count(&resp.CurrentParking)

	cardsDB := db.Model(&models.MonthlyCard{})
	cardsDB = scopedLotID(c, cardsDB, "parking_lot_id")
	today := time.Now()
	cardsDB.Session(&gorm.Session{}).Where("status = ? AND start_date <= ? AND end_date >= ?", "active", today, today).
		Count(&resp.ActiveMonthlyCards)

	todayStr := "DATE(created_at) = CURRENT_DATE"
	recToday := recordsDB.Session(&gorm.Session{})
	recToday.Where(todayStr).Count(&resp.TodayEntry)

	exitDB := db.Model(&models.ParkingRecord{})
	exitDB = scopedLotID(c, exitDB, "parking_lot_id")
	exitDB.Where("status = ? AND DATE(exit_time) = CURRENT_DATE", "completed").
		Count(&resp.TodayExit)

	payDB := db.Model(&models.PaymentRecord{})
	payDB = scopedLotID(c, payDB, "parking_lot_id")

	resp.TodayParkingIncome = sumAmount(payDB.Session(&gorm.Session{}), "parking", todayStr)
	resp.TodayMonthlyIncome = sumAmount(payDB.Session(&gorm.Session{}), "monthly", todayStr)
	resp.TodayTotalIncome = resp.TodayParkingIncome + resp.TodayMonthlyIncome

	monthStart := time.Now().Format("2006-01") + "-01"
	monthCond := "DATE(created_at) >= '" + monthStart + "'"
	recMonth := recordsDB.Session(&gorm.Session{})
	recMonth.Where(monthCond).Count(&resp.MonthEntry)

	exitMon := db.Model(&models.ParkingRecord{})
	exitMon = scopedLotID(c, exitMon, "parking_lot_id")
	exitMon.Where("status = ? AND DATE(exit_time) >= ?", "completed", monthStart).
		Count(&resp.MonthExit)

	resp.MonthParkingIncome = sumAmount(payDB.Session(&gorm.Session{}), "parking", monthCond)
	resp.MonthMonthlyIncome = sumAmount(payDB.Session(&gorm.Session{}), "monthly", monthCond)
	resp.MonthTotalIncome = resp.MonthParkingIncome + resp.MonthMonthlyIncome

	utils.OK(c, resp)
}

type IncomeTrendItem struct {
	Date          string  `json:"date"`
	ParkingIncome float64 `json:"parking_income"`
	MonthlyIncome float64 `json:"monthly_income"`
	TotalIncome   float64 `json:"total_income"`
	EntryCount    int64   `json:"entry_count"`
	ExitCount     int64   `json:"exit_count"`
}

func atoiSafe(s string, def int) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return def
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

func (h *ReportHandler) IncomeTrend(c *gin.Context) {
	days := 7
	if d := c.Query("days"); d != "" {
		n := atoiSafe(d, 0)
		if n > 0 && n <= 365 {
			days = n
		}
	}

	result := make([]IncomeTrendItem, days)
	today := time.Now()
	lotCond := ""
	params := []interface{}{}

	if !middleware.IsSuperAdmin(c) {
		pid := middleware.GetParkingLotID(c)
		if pid != nil {
			lotCond = " AND parking_lot_id = ? "
			params = append(params, *pid)
		} else {
			utils.OK(c, result)
			return
		}
	}

	for i := 0; i < days; i++ {
		date := today.AddDate(0, 0, -(days - 1 - i))
		dateStr := date.Format("2006-01-02")
		item := IncomeTrendItem{Date: dateStr}

		parkParams := append([]interface{}{"parking", "success", dateStr}, params...)
		type r1 struct{ S float64 }
		var rr1 r1
		utils.DB.Raw("SELECT COALESCE(SUM(amount),0) as s FROM payment_records WHERE payment_type=? AND status=? AND DATE(created_at)=?"+lotCond,
			parkParams...).Scan(&rr1)
		item.ParkingIncome = rr1.S

		monParams := append([]interface{}{"monthly", "success", dateStr}, params...)
		var rr2 r1
		utils.DB.Raw("SELECT COALESCE(SUM(amount),0) as s FROM payment_records WHERE payment_type=? AND status=? AND DATE(created_at)=?"+lotCond,
			monParams...).Scan(&rr2)
		item.MonthlyIncome = rr2.S
		item.TotalIncome = item.ParkingIncome + item.MonthlyIncome

		entryParams := append([]interface{}{dateStr}, params...)
		type r2 struct{ C int64 }
		var cr r2
		utils.DB.Raw("SELECT COUNT(*) as c FROM parking_records WHERE DATE(entry_time)=?"+lotCond, entryParams...).Scan(&cr)
		item.EntryCount = cr.C

		exitParams := append([]interface{}{"completed", dateStr}, params...)
		utils.DB.Raw("SELECT COUNT(*) as c FROM parking_records WHERE status=? AND DATE(exit_time)=?"+lotCond, exitParams...).Scan(&cr)
		item.ExitCount = cr.C

		result[i] = item
	}

	utils.OK(c, result)
}

type TopItem struct {
	Plate  string  `json:"vehicle_plate"`
	Count  int64   `json:"count"`
	Amount float64 `json:"amount"`
}

func (h *ReportHandler) TopVehicles(c *gin.Context) {
	limit := 10
	if l := c.Query("limit"); l != "" {
		n := atoiSafe(l, 0)
		if n > 0 && n <= 100 {
			limit = n
		}
	}
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	if startDate == "" {
		startDate = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	lotCond := ""
	params := []interface{}{startDate, endDate + " 23:59:59"}

	if !middleware.IsSuperAdmin(c) {
		pid := middleware.GetParkingLotID(c)
		if pid != nil {
			lotCond = " AND parking_lot_id = ? "
			params = append(params, *pid)
		}
	}

	rows := []struct {
		Plate  string
		Count  int64
		Amount float64
	}{}
	utils.DB.Raw(
		`SELECT vehicle_plate as plate, COUNT(*) as count, COALESCE(SUM(paid_amount),0) as amount
		 FROM parking_records
		 WHERE entry_time >= ? AND entry_time <= ? `+lotCond+`
		 GROUP BY vehicle_plate
		 ORDER BY count DESC
		 LIMIT ?`,
		append(params, limit)...,
	).Scan(&rows)

	list := make([]TopItem, len(rows))
	for i, r := range rows {
		list[i] = TopItem{Plate: r.Plate, Count: r.Count, Amount: r.Amount}
	}
	utils.OK(c, list)
}

type PaymentStats struct {
	CashCount     int64   `json:"cash_count"`
	CashAmount    float64 `json:"cash_amount"`
	WechatCount   int64   `json:"wechat_count"`
	WechatAmount  float64 `json:"wechat_amount"`
	AlipayCount   int64   `json:"alipay_count"`
	AlipayAmount  float64 `json:"alipay_amount"`
	CardCount     int64   `json:"card_count"`
	CardAmount    float64 `json:"card_amount"`
	MonthlyCount  int64   `json:"monthly_count"`
	MonthlyAmount float64 `json:"monthly_amount"`
	TotalCount    int64   `json:"total_count"`
	TotalAmount   float64 `json:"total_amount"`
}

func (h *ReportHandler) PaymentStats(c *gin.Context) {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	if startDate == "" {
		startDate = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	lotCond := ""
	params := []interface{}{startDate, endDate + " 23:59:59", "success"}

	if !middleware.IsSuperAdmin(c) {
		pid := middleware.GetParkingLotID(c)
		if pid != nil {
			lotCond = " AND parking_lot_id = ? "
			params = append(params, *pid)
		}
	}

	type row struct {
		Method string
		Count  int64
		Amount float64
	}
	var rows []row
	utils.DB.Raw(
		`SELECT payment_method as method, COUNT(*) as count, COALESCE(SUM(amount),0) as amount
		 FROM payment_records
		 WHERE created_at >= ? AND created_at <= ? AND status = ? `+lotCond+`
		 GROUP BY payment_method`,
		params...,
	).Scan(&rows)

	var stats PaymentStats
	for _, r := range rows {
		switch r.Method {
		case "cash":
			stats.CashCount = r.Count
			stats.CashAmount = r.Amount
		case "wechat":
			stats.WechatCount = r.Count
			stats.WechatAmount = r.Amount
		case "alipay":
			stats.AlipayCount = r.Count
			stats.AlipayAmount = r.Amount
		case "card":
			stats.CardCount = r.Count
			stats.CardAmount = r.Amount
		case "monthly":
			stats.MonthlyCount = r.Count
			stats.MonthlyAmount = r.Amount
		}
		stats.TotalCount += r.Count
		stats.TotalAmount += r.Amount
	}
	utils.OK(c, stats)
}

type ZoneSpace struct {
	Total     int `json:"total"`
	Occupied  int `json:"occupied"`
	Available int `json:"available"`
}
type TypeSpace struct {
	Total     int `json:"total"`
	Occupied  int `json:"occupied"`
	Available int `json:"available"`
}
type SpaceStats struct {
	TotalSpaces   int                  `json:"total_spaces"`
	Available     int                  `json:"available"`
	Occupied      int                  `json:"occupied"`
	Reserved      int                  `json:"reserved"`
	Maintenance   int                  `json:"maintenance"`
	OccupancyRate float64              `json:"occupancy_rate"`
	ByZone        map[string]ZoneSpace `json:"by_zone"`
	ByType        map[string]TypeSpace `json:"by_type"`
}

func (h *ReportHandler) SpaceStats(c *gin.Context) {
	lotID, ok := getTargetLotID(c)
	if !ok {
		return
	}
	var spaces []models.ParkingSpace
	utils.DB.Where("parking_lot_id = ?", lotID).Find(&spaces)

	stats := SpaceStats{
		TotalSpaces: len(spaces),
		ByZone:      make(map[string]ZoneSpace),
		ByType:      make(map[string]TypeSpace),
	}
	for _, s := range spaces {
		switch s.Status {
		case "available":
			stats.Available++
		case "occupied":
			stats.Occupied++
		case "reserved":
			stats.Reserved++
		case "maintenance":
			stats.Maintenance++
		}
		zs := stats.ByZone[s.Zone]
		zs.Total++
		if s.Status == "occupied" {
			zs.Occupied++
		} else if s.Status == "available" {
			zs.Available++
		}
		stats.ByZone[s.Zone] = zs

		ts := stats.ByType[s.Type]
		ts.Total++
		if s.Status == "occupied" {
			ts.Occupied++
		} else if s.Status == "available" {
			ts.Available++
		}
		stats.ByType[s.Type] = ts
	}
	if stats.TotalSpaces > 0 {
		stats.OccupancyRate = float64(stats.Occupied) / float64(stats.TotalSpaces) * 100
	}
	utils.OK(c, stats)
}

type MonthlyCardStats struct {
	Total       int64            `json:"total"`
	Active      int64            `json:"active"`
	Expired     int64            `json:"expired"`
	Suspended   int64            `json:"suspended"`
	Expiring7   int64            `json:"expiring_7_days"`
	Expiring30  int64            `json:"expiring_30_days"`
	TotalIncome float64          `json:"total_income"`
	MonthIncome float64          `json:"month_income"`
	ByPlanType  map[string]int64 `json:"by_plan_type"`
}

func (h *ReportHandler) MonthlyCardStats(c *gin.Context) {
	cardsDB := utils.DB.Model(&models.MonthlyCard{})
	cardsDB = scopedLotID(c, cardsDB, "parking_lot_id")

	var stats MonthlyCardStats
	stats.ByPlanType = make(map[string]int64)

	cardsDB.Session(&gorm.Session{}).Count(&stats.Total)
	cardsDB.Session(&gorm.Session{}).Where("status = ?", "active").Count(&stats.Active)
	cardsDB.Session(&gorm.Session{}).Where("status = ?", "expired").Count(&stats.Expired)
	cardsDB.Session(&gorm.Session{}).Where("status = ?", "suspended").Count(&stats.Suspended)

	today := time.Now()
	d7 := today.AddDate(0, 0, 7)
	cardsDB.Session(&gorm.Session{}).Where("status = ? AND end_date <= ? AND end_date >= ?", "active", d7, today).Count(&stats.Expiring7)
	d30 := today.AddDate(0, 0, 30)
	cardsDB.Session(&gorm.Session{}).Where("status = ? AND end_date <= ? AND end_date >= ?", "active", d30, today).Count(&stats.Expiring30)

	payDB := utils.DB.Model(&models.PaymentRecord{})
	payDB = scopedLotID(c, payDB, "parking_lot_id")
	stats.TotalIncome = sumAmount(payDB.Session(&gorm.Session{}), "monthly", "1=1")
	monthStart := time.Now().Format("2006-01") + "-01"
	stats.MonthIncome = sumAmount(payDB.Session(&gorm.Session{}), "monthly",
		"DATE(created_at) >= '"+monthStart+"'")

	ptRows := []struct {
		Plan  string
		Count int64
	}{}
	cardsDB.Session(&gorm.Session{}).Select("plan_type as plan, COUNT(*) as count").Group("plan_type").Scan(&ptRows)
	for _, r := range ptRows {
		stats.ByPlanType[r.Plan] = r.Count
	}
	utils.OK(c, stats)
}
