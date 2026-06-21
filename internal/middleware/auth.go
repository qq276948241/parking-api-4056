package middleware

import (
	"parking-system/internal/utils"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	CtxAdminID      = "admin_id"
	CtxUsername     = "username"
	CtxRole         = "role"
	CtxParkingLotID = "parking_lot_id"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			utils.Unauthorized(c, "缺少Authorization头")
			c.Abort()
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			utils.Unauthorized(c, "Authorization格式错误")
			c.Abort()
			return
		}
		claims, err := utils.ParseToken(parts[1])
		if err != nil {
			utils.Unauthorized(c, "Token无效或已过期")
			c.Abort()
			return
		}
		c.Set(CtxAdminID, claims.AdminID)
		c.Set(CtxUsername, claims.Username)
		c.Set(CtxRole, claims.Role)
		c.Set(CtxParkingLotID, claims.ParkingLotID)
		c.Next()
	}
}

func SuperAdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get(CtxRole)
		if role != "super_admin" {
			utils.Forbidden(c, "需要超级管理员权限")
			c.Abort()
			return
		}
		c.Next()
	}
}

func GetAdminID(c *gin.Context) uuid.UUID {
	v, ok := c.Get(CtxAdminID)
	if !ok {
		return uuid.Nil
	}
	return v.(uuid.UUID)
}

func GetRole(c *gin.Context) string {
	v, ok := c.Get(CtxRole)
	if !ok {
		return ""
	}
	return v.(string)
}

func GetParkingLotID(c *gin.Context) *uuid.UUID {
	v, ok := c.Get(CtxParkingLotID)
	if !ok || v == nil {
		return nil
	}
	pid := v.(uuid.UUID)
	return &pid
}

func IsSuperAdmin(c *gin.Context) bool {
	return GetRole(c) == "super_admin"
}
