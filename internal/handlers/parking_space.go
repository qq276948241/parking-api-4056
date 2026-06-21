package handlers

import (
	"parking-system/internal/middleware"
	"parking-system/internal/models"
	"parking-system/internal/utils"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ParkingSpaceHandler struct{}

func NewParkingSpaceHandler() *ParkingSpaceHandler {
	return &ParkingSpaceHandler{}
}

type SpaceCreateReq struct {
	SpaceNumber string `json:"space_number" binding:"required"`
	Zone        string `json:"zone"`
	Type        string `json:"type" binding:"required,oneof=standard reserved disabled"`
	Status      string `json:"status" binding:"omitempty,oneof=available occupied reserved maintenance"`
}

type SpaceUpdateReq struct {
	SpaceNumber string  `json:"space_number"`
	Zone        *string `json:"zone"`
	Type        string  `json:"type" binding:"omitempty,oneof=standard reserved disabled"`
	Status      string  `json:"status" binding:"omitempty,oneof=available occupied reserved maintenance"`
	VehiclePlate string `json:"vehicle_plate"`
}

type BatchCreateReq struct {
	Prefix    string `json:"prefix"`
	StartNum  int    `json:"start_num" binding:"required,min=1"`
	EndNum    int    `json:"end_num" binding:"required,gtefield=StartNum"`
	Zone      string `json:"zone"`
	Type      string `json:"type" binding:"required,oneof=standard reserved disabled"`
	PaddingLen int   `json:"padding_len" binding:"omitempty,min=0,max=6"`
}

func (h *ParkingSpaceHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	keyword := c.Query("keyword")
	zone := c.Query("zone")
	status := c.Query("status")
	typ := c.Query("type")

	db := utils.DB.Model(&models.ParkingSpace{})
	db = scopedLotID(c, db, "parking_lot_id")

	if keyword != "" {
		db = db.Where("space_number LIKE ? OR vehicle_plate LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if zone != "" {
		db = db.Where("zone = ?", zone)
	}
	if status != "" {
		db = db.Where("status = ?", status)
	}
	if typ != "" {
		db = db.Where("type = ?", typ)
	}

	var total int64
	db.Count(&total)
	var list []models.ParkingSpace
	db.Order("zone, space_number").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list)
	utils.OKPaged(c, list, total, page, pageSize)
}

func (h *ParkingSpaceHandler) Create(c *gin.Context) {
	lotID, ok := getTargetLotID(c)
	if !ok {
		return
	}
	var req SpaceCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	status := "available"
	if req.Status != "" {
		status = req.Status
	}
	space := models.ParkingSpace{
		ParkingLotID: lotID,
		SpaceNumber:  req.SpaceNumber,
		Zone:         req.Zone,
		Type:         req.Type,
		Status:       status,
	}
	if err := utils.DB.Create(&space).Error; err != nil {
		utils.InternalError(c, "创建车位失败: 编号可能已存在")
		return
	}
	utils.OK(c, space)
}

func (h *ParkingSpaceHandler) BatchCreate(c *gin.Context) {
	lotID, ok := getTargetLotID(c)
	if !ok {
		return
	}
	var req BatchCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	padding := req.PaddingLen
	spaces := make([]models.ParkingSpace, 0, req.EndNum-req.StartNum+1)
	for i := req.StartNum; i <= req.EndNum; i++ {
		num := strconv.Itoa(i)
		if padding > len(num) {
			num = padLeft(num, "0", padding)
		}
		spaces = append(spaces, models.ParkingSpace{
			ParkingLotID: lotID,
			SpaceNumber:  req.Prefix + num,
			Zone:         req.Zone,
			Type:         req.Type,
			Status:       "available",
		})
	}
	tx := utils.DB.Begin()
	created := 0
	for _, sp := range spaces {
		if err := tx.Create(&sp).Error; err == nil {
			created++
		}
	}
	tx.Commit()
	utils.OK(c, gin.H{"created": created, "total": len(spaces)})
}

func (h *ParkingSpaceHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	var space models.ParkingSpace
	if err := utils.DB.First(&space, id).Error; err != nil {
		utils.NotFound(c, "车位不存在")
		return
	}
	if err := checkLotAccess(c, space.ParkingLotID); err != nil {
		utils.Forbidden(c, err.Error())
		return
	}
	utils.OK(c, space)
}

func (h *ParkingSpaceHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	var space models.ParkingSpace
	if err := utils.DB.First(&space, id).Error; err != nil {
		utils.NotFound(c, "车位不存在")
		return
	}
	if err := checkLotAccess(c, space.ParkingLotID); err != nil {
		utils.Forbidden(c, err.Error())
		return
	}
	var req SpaceUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if req.SpaceNumber != "" {
		space.SpaceNumber = req.SpaceNumber
	}
	if req.Zone != nil {
		space.Zone = *req.Zone
	}
	if req.Type != "" {
		space.Type = req.Type
	}
	if req.Status != "" {
		space.Status = req.Status
	}
	if req.VehiclePlate != "" {
		space.VehiclePlate = req.VehiclePlate
	}
	if err := utils.DB.Save(&space).Error; err != nil {
		utils.InternalError(c, "更新车位失败")
		return
	}
	utils.OK(c, space)
}

func (h *ParkingSpaceHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.BadRequest(c, "ID格式错误")
		return
	}
	var space models.ParkingSpace
	if err := utils.DB.First(&space, id).Error; err != nil {
		utils.NotFound(c, "车位不存在")
		return
	}
	if err := checkLotAccess(c, space.ParkingLotID); err != nil {
		utils.Forbidden(c, err.Error())
		return
	}
	result := utils.DB.Delete(&models.ParkingSpace{}, id)
	if result.Error != nil {
		utils.InternalError(c, "删除车位失败")
		return
	}
	utils.OK(c, nil)
}

func getTargetLotID(c *gin.Context) (uuid.UUID, bool) {
	lidStr := c.Query("parking_lot_id")
	if lidStr == "" {
		lidStr = c.PostForm("parking_lot_id")
	}
	var lotID uuid.UUID
	if lidStr != "" {
		pid, err := uuid.Parse(lidStr)
		if err != nil {
			utils.BadRequest(c, "停车场ID格式错误")
			return uuid.Nil, false
		}
		if err := checkLotAccess(c, pid); err != nil {
			utils.Forbidden(c, err.Error())
			return uuid.Nil, false
		}
		lotID = pid
	} else {
		pid := middleware.GetParkingLotID(c)
		if pid == nil {
			utils.BadRequest(c, "缺少parking_lot_id参数")
			return uuid.Nil, false
		}
		lotID = *pid
	}
	return lotID, true
}

func checkLotAccess(c *gin.Context, lotID uuid.UUID) error {
	if middleware.IsSuperAdmin(c) {
		return nil
	}
	pid := middleware.GetParkingLotID(c)
	if pid == nil || lotID != *pid {
		return errNoAccess
	}
	return nil
}

var errNoAccess = errorString("无权访问该停车场数据")

type errorString string

func (e errorString) Error() string { return string(e) }

func padLeft(s, pad string, length int) string {
	for len(s) < length {
		s = pad + s
	}
	return s
}
