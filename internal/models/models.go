package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Base struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (b *Base) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

type Admin struct {
	Base
	Username     string    `gorm:"size:50;uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"`
	RealName     string    `gorm:"size:50" json:"real_name"`
	Role         string    `gorm:"size:20;not null;default:admin" json:"role"`
	ParkingLotID *uuid.UUID `gorm:"type:uuid" json:"parking_lot_id"`
	IsActive     bool      `gorm:"not null;default:true" json:"is_active"`
}

func (Admin) TableName() string { return "admins" }

type ParkingLot struct {
	Base
	Name         string    `gorm:"size:100;not null" json:"name"`
	Address      string    `gorm:"type:text" json:"address"`
	ContactPhone string    `gorm:"size:20" json:"contact_phone"`
	TotalSpaces  int       `gorm:"not null;default:0" json:"total_spaces"`
	HourlyRate   float64   `gorm:"type:decimal(10,2);not null;default:5.00" json:"hourly_rate"`
	DailyMax     float64   `gorm:"type:decimal(10,2);not null;default:50.00" json:"daily_max"`
	FreeMinutes  int       `gorm:"not null;default:30" json:"free_minutes"`
	IsActive     bool      `gorm:"not null;default:true" json:"is_active"`
}

func (ParkingLot) TableName() string { return "parking_lots" }

type ParkingSpace struct {
	Base
	ParkingLotID uuid.UUID `gorm:"type:uuid;not null;index" json:"parking_lot_id"`
	SpaceNumber  string    `gorm:"size:20;not null" json:"space_number"`
	Zone         string    `gorm:"size:50" json:"zone"`
	Type         string    `gorm:"size:20;not null;default:standard" json:"type"`
	Status       string    `gorm:"size:20;not null;default:available;index" json:"status"`
	VehiclePlate string    `gorm:"size:20" json:"vehicle_plate"`
}

func (ParkingSpace) TableName() string { return "parking_spaces" }

type ParkingRecord struct {
	Base
	ParkingLotID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"parking_lot_id"`
	SpaceID       *uuid.UUID `gorm:"type:uuid" json:"space_id"`
	VehiclePlate  string     `gorm:"size:20;not null;index" json:"vehicle_plate"`
	VehicleType   string     `gorm:"size:20;not null;default:car" json:"vehicle_type"`
	EntryTime     time.Time  `gorm:"not null" json:"entry_time"`
	ExitTime      *time.Time `json:"exit_time"`
	DurationMin   int        `gorm:"default:0" json:"duration_minutes"`
	HourlyRate    float64    `gorm:"type:decimal(10,2);not null;default:0" json:"hourly_rate"`
	Discount      float64    `gorm:"type:decimal(10,2);not null;default:0" json:"discount"`
	TotalAmount   float64    `gorm:"type:decimal(10,2);not null;default:0" json:"total_amount"`
	PaidAmount    float64    `gorm:"type:decimal(10,2);not null;default:0" json:"paid_amount"`
	PaymentStatus string     `gorm:"size:20;not null;default:unpaid" json:"payment_status"`
	PaymentMethod string     `gorm:"size:20" json:"payment_method"`
	MonthlyCardID *uuid.UUID `gorm:"type:uuid" json:"monthly_card_id"`
	IsMonthly     bool       `gorm:"not null;default:false" json:"is_monthly"`
	Status        string     `gorm:"size:20;not null;default:parking;index" json:"status"`
	Remarks       string     `gorm:"type:text" json:"remarks"`
}

func (ParkingRecord) TableName() string { return "parking_records" }

type MonthlyCard struct {
	Base
	ParkingLotID uuid.UUID  `gorm:"type:uuid;not null;index" json:"parking_lot_id"`
	CardNumber   string     `gorm:"size:50;uniqueIndex;not null" json:"card_number"`
	VehiclePlate string     `gorm:"size:20;not null;index" json:"vehicle_plate"`
	OwnerName    string     `gorm:"size:50" json:"owner_name"`
	OwnerPhone   string     `gorm:"size:20" json:"owner_phone"`
	PlanName     string     `gorm:"size:50;not null" json:"plan_name"`
	PlanType     string     `gorm:"size:20;not null;default:monthly" json:"plan_type"`
	Price        float64    `gorm:"type:decimal(10,2);not null" json:"price"`
	StartDate    time.Time  `gorm:"type:date;not null" json:"start_date"`
	EndDate      time.Time  `gorm:"type:date;not null;index" json:"end_date"`
	Status       string     `gorm:"size:20;not null;default:active;index" json:"status"`
	PaidAmount   float64    `gorm:"type:decimal(10,2);not null;default:0" json:"paid_amount"`
	PaymentMethod string    `gorm:"size:20" json:"payment_method"`
	Remarks      string     `gorm:"type:text" json:"remarks"`
}

func (MonthlyCard) TableName() string { return "monthly_cards" }

type PaymentRecord struct {
	Base
	ParkingLotID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"parking_lot_id"`
	ParkingRecordID *uuid.UUID `gorm:"type:uuid" json:"parking_record_id"`
	MonthlyCardID  *uuid.UUID `gorm:"type:uuid" json:"monthly_card_id"`
	PaymentType    string     `gorm:"size:20;not null" json:"payment_type"`
	Amount         float64    `gorm:"type:decimal(10,2);not null" json:"amount"`
	PaymentMethod  string     `gorm:"size:20;not null" json:"payment_method"`
	TransactionNo  string     `gorm:"size:100" json:"transaction_no"`
	OperatorID     *uuid.UUID `gorm:"type:uuid" json:"operator_id"`
	Status         string     `gorm:"size:20;not null;default:success" json:"status"`
	Remarks        string     `gorm:"type:text" json:"remarks"`
}

func (PaymentRecord) TableName() string { return "payment_records" }
