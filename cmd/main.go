package main

import (
	"fmt"
	"net/http"
	"parking-system/configs"
	"parking-system/internal/handlers"
	"parking-system/internal/middleware"
	"parking-system/internal/utils"

	"github.com/gin-gonic/gin"
)

func main() {
	if err := configs.Load(); err != nil {
		panic("加载配置失败: " + err.Error())
	}

	if err := utils.InitDB(); err != nil {
		panic("连接数据库失败: " + err.Error())
	}
	fmt.Println("数据库连接成功")

	gin.SetMode(configs.AppConfig.ServerMode)
	r := gin.Default()

	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	authHandler := handlers.NewAuthHandler()
	adminHandler := handlers.NewAdminHandler()
	lotHandler := handlers.NewParkingLotHandler()
	spaceHandler := handlers.NewParkingSpaceHandler()
	recordHandler := handlers.NewParkingRecordHandler()
	cardHandler := handlers.NewMonthlyCardHandler()
	reportHandler := handlers.NewReportHandler()

	r.GET("/health", func(c *gin.Context) {
		utils.OK(c, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1")
	{
		auth := api.Group("/auth")
		{
			auth.POST("/login", authHandler.Login)
		}

		authorized := api.Group("")
		authorized.Use(middleware.AuthMiddleware())
		{
			auth2 := authorized.Group("/auth")
			{
				auth2.GET("/profile", authHandler.GetProfile)
				auth2.PUT("/password", authHandler.ChangePassword)
			}

			admins := authorized.Group("/admins")
			admins.Use(middleware.SuperAdminOnly())
			{
				admins.GET("", adminHandler.List)
				admins.POST("", adminHandler.Create)
				admins.GET("/:id", adminHandler.Get)
				admins.PUT("/:id", adminHandler.Update)
				admins.DELETE("/:id", adminHandler.Delete)
			}

			lots := authorized.Group("/parking-lots")
			{
				lots.GET("", lotHandler.List)
				lots.POST("", middleware.SuperAdminOnly(), lotHandler.Create)
				lots.GET("/:id", lotHandler.Get)
				lots.PUT("/:id", lotHandler.Update)
				lots.DELETE("/:id", middleware.SuperAdminOnly(), lotHandler.Delete)
				lots.GET("/:id/stats", lotHandler.Stats)
			}

			spaces := authorized.Group("/parking-spaces")
			{
				spaces.GET("", spaceHandler.List)
				spaces.POST("", spaceHandler.Create)
				spaces.POST("/batch", spaceHandler.BatchCreate)
				spaces.GET("/:id", spaceHandler.Get)
				spaces.PUT("/:id", spaceHandler.Update)
				spaces.DELETE("/:id", spaceHandler.Delete)
			}

			records := authorized.Group("/parking-records")
			{
				records.GET("", recordHandler.List)
				records.GET("/current", recordHandler.CurrentParking)
				records.GET("/calc-fee", recordHandler.CalcFee)
				records.POST("/entry", recordHandler.Entry)
				records.GET("/:id", recordHandler.Get)
				records.PUT("/:id", recordHandler.Update)
				records.POST("/:id/exit", recordHandler.Exit)
				records.POST("/:id/pay", recordHandler.Pay)
			}

			cards := authorized.Group("/monthly-cards")
			{
				cards.GET("", cardHandler.List)
				cards.GET("/check", cardHandler.Check)
				cards.POST("", cardHandler.Create)
				cards.GET("/:id", cardHandler.Get)
				cards.PUT("/:id", cardHandler.Update)
				cards.POST("/:id/renew", cardHandler.Renew)
				cards.DELETE("/:id", cardHandler.Delete)
			}

			reports := authorized.Group("/reports")
			{
				reports.GET("/overview", reportHandler.Overview)
				reports.GET("/income-trend", reportHandler.IncomeTrend)
				reports.GET("/top-vehicles", reportHandler.TopVehicles)
				reports.GET("/payment-stats", reportHandler.PaymentStats)
				reports.GET("/space-stats", reportHandler.SpaceStats)
				reports.GET("/monthly-card-stats", reportHandler.MonthlyCardStats)
			}
		}
	}

	addr := ":" + configs.AppConfig.ServerPort
	fmt.Println("服务启动于", addr)
	if err := r.Run(addr); err != nil {
		panic("启动服务失败: " + err.Error())
	}
}
