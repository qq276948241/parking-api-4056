package utils

import (
	"parking-system/configs"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB() error {
	var err error
	DB, err = gorm.Open(postgres.Open(configs.AppConfig.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	return err
}
