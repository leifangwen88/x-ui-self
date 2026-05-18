package service

import (
	"fmt"
	"time"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/util/common"
	"x-ui/xray"

	"gorm.io/gorm"
)

type InboundService struct {
}

func (s *InboundService) GetInbounds(userId int) ([]*model.Inbound, error) {
	db := database.GetDB()
	var inbounds []*model.Inbound
	err := db.Model(model.Inbound{}).Where("user_id = ?", userId).Find(&inbounds).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	return inbounds, nil
}

func (s *InboundService) GetAllInbounds() ([]*model.Inbound, error) {
	db := database.GetDB()
	var inbounds []*model.Inbound
	err := db.Model(model.Inbound{}).Find(&inbounds).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	return inbounds, nil
}

func (s *InboundService) checkPortExist(port int, ignoreId int) (bool, error) {
	db := database.GetDB()
	db = db.Model(model.Inbound{}).Where("port = ?", port)
	if ignoreId > 0 {
		db = db.Where("id != ?", ignoreId)
	}
	var count int64
	err := db.Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *InboundService) AddInbound(inbound *model.Inbound) error {
	exist, err := s.checkPortExist(inbound.Port, 0)
	if err != nil {
		return err
	}
	if exist {
		return common.NewError("端口已存在:", inbound.Port)
	}
	if err = s.checkSocksProxyId(inbound.SocksProxyId); err != nil {
		return err
	}
	if err = s.checkGameId(inbound.GameId); err != nil {
		return err
	}
	if inbound.RotationPolicy == "" {
		inbound.RotationPolicy = model.RotationPolicyPreferUnusedUnbanned
	}
	db := database.GetDB()
	if err := db.Save(inbound).Error; err != nil {
		return err
	}
	EmitInboundUpsert(inbound)
	return nil
}

func (s *InboundService) AddInbounds(inbounds []*model.Inbound) error {
	for _, inbound := range inbounds {
		exist, err := s.checkPortExist(inbound.Port, 0)
		if err != nil {
			return err
		}
		if exist {
			return common.NewError("端口已存在:", inbound.Port)
		}
	}

	db := database.GetDB()
	tx := db.Begin()
	var err error
	defer func() {
		if err == nil {
			tx.Commit()
		} else {
			tx.Rollback()
		}
	}()

	for _, inbound := range inbounds {
		err = tx.Save(inbound).Error
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *InboundService) DelInbound(id int) error {
	ib, err := s.GetInbound(id)
	if err != nil {
		return err
	}
	port := ib.Port
	db := database.GetDB()
	if err := db.Delete(model.Inbound{}, id).Error; err != nil {
		return err
	}
	EmitInboundDeletePort(port)
	return nil
}

func (s *InboundService) GetInbound(id int) (*model.Inbound, error) {
	db := database.GetDB()
	inbound := &model.Inbound{}
	err := db.Model(model.Inbound{}).First(inbound, id).Error
	if err != nil {
		return nil, err
	}
	return inbound, nil
}

func (s *InboundService) UpdateInbound(inbound *model.Inbound) error {
	exist, err := s.checkPortExist(inbound.Port, inbound.Id)
	if err != nil {
		return err
	}
	if exist {
		return common.NewError("端口已存在:", inbound.Port)
	}

	oldInbound, err := s.GetInbound(inbound.Id)
	if err != nil {
		return err
	}
	prevPort := oldInbound.Port
	oldInbound.Up = inbound.Up
	oldInbound.Down = inbound.Down
	oldInbound.Total = inbound.Total
	oldInbound.Remark = inbound.Remark
	oldInbound.Enable = inbound.Enable
	oldInbound.ExpiryTime = inbound.ExpiryTime
	oldInbound.Listen = inbound.Listen
	oldInbound.Port = inbound.Port
	oldInbound.Protocol = inbound.Protocol
	oldInbound.Settings = inbound.Settings
	oldInbound.StreamSettings = inbound.StreamSettings
	oldInbound.Sniffing = inbound.Sniffing
	if err = s.checkSocksProxyId(inbound.SocksProxyId); err != nil {
		return err
	}
	if err = s.checkGameId(inbound.GameId); err != nil {
		return err
	}
	oldInbound.SocksProxyId = inbound.SocksProxyId
	oldInbound.GameId = inbound.GameId
	oldInbound.RotationEnable = inbound.RotationEnable
	if inbound.RotationPolicy != "" {
		oldInbound.RotationPolicy = inbound.RotationPolicy
	}
	oldInbound.Tag = fmt.Sprintf("inbound-%v", inbound.Port)

	db := database.GetDB()
	if err := db.Save(oldInbound).Error; err != nil {
		return err
	}
	if globalPanelSync != nil && !globalPanelSync.IsApplying() {
		if prevPort != inbound.Port {
			EmitInboundDeletePort(prevPort)
		}
		EmitInboundUpsert(oldInbound)
	}
	return nil
}

func (s *InboundService) checkSocksProxyId(socksProxyId int) error {
	if socksProxyId <= 0 {
		return nil
	}
	socksService := SocksProxyService{}
	_, err := socksService.GetById(socksProxyId)
	if err != nil {
		return common.NewError("SOCKS5 不存在:", socksProxyId)
	}
	return nil
}

func (s *InboundService) AddTraffic(traffics []*xray.Traffic) (err error) {
	if len(traffics) == 0 {
		return nil
	}
	db := database.GetDB()
	db = db.Model(model.Inbound{})
	tx := db.Begin()
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()
	for _, traffic := range traffics {
		if traffic.IsInbound {
			err = tx.Where("tag = ?", traffic.Tag).
				UpdateColumn("up", gorm.Expr("up + ?", traffic.Up)).
				UpdateColumn("down", gorm.Expr("down + ?", traffic.Down)).
				Error
			if err != nil {
				return
			}
		}
	}
	return
}

func (s *InboundService) UpdateGameId(id int, gameId int) error {
	if err := s.checkGameId(gameId); err != nil {
		return err
	}
	inbound, err := s.GetInbound(id)
	if err != nil {
		return err
	}
	inbound.GameId = gameId
	db := database.GetDB()
	result := db.Model(inbound).Update("game_id", gameId)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return common.NewError("入站不存在:", id)
	}
	if globalPanelSync != nil && !globalPanelSync.IsApplying() {
		EmitInboundUpsertById(id)
	}
	return nil
}

func (s *InboundService) UpdateSocksProxyId(id int, socksProxyId int) error {
	return s.updateSocksProxyId(id, socksProxyId, true)
}

func (s *InboundService) updateSocksProxyId(id int, socksProxyId int, emitSync bool) error {
	inbound, err := s.GetInbound(id)
	if err != nil {
		return err
	}
	if err := s.checkSocksProxyId(socksProxyId); err != nil {
		return err
	}
	if socksProxyId > 0 {
		inbounds, err := s.GetAllInbounds()
		if err != nil {
			return err
		}
		for _, ib := range inbounds {
			if ib.Id != id && ib.SocksProxyId == socksProxyId {
				return common.NewError("该 SOCKS 已绑定其他入站")
			}
		}
	}
	db := database.GetDB()
	result := db.Model(model.Inbound{}).Where("id = ?", id).Update("socks_proxy_id", socksProxyId)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return common.NewError("入站不存在:", id)
	}
	if socksProxyId > 0 && inbound.GameId > 0 {
		socksGame := SocksGameService{}
		_ = socksGame.RecordUsage(socksProxyId, inbound.GameId)
	}
	if emitSync && globalPanelSync != nil && !globalPanelSync.IsApplying() {
		EmitInboundUpsertById(id)
	}
	return nil
}

func (s *InboundService) checkGameId(gameId int) error {
	if gameId <= 0 {
		return nil
	}
	gameService := GameService{}
	_, err := gameService.GetById(gameId)
	if err != nil {
		return common.NewError("游戏不存在:", gameId)
	}
	return nil
}

func (s *InboundService) ResetAllTraffic() error {
	db := database.GetDB()
	return db.Model(model.Inbound{}).Where("1 = 1").Updates(map[string]interface{}{
		"up":   0,
		"down": 0,
	}).Error
}

func (s *InboundService) DisableInvalidInbounds() (int64, error) {
	db := database.GetDB()
	now := time.Now().Unix() * 1000
	result := db.Model(model.Inbound{}).
		Where("((total > 0 and up + down >= total) or (expiry_time > 0 and expiry_time <= ?)) and enable = ?", now, true).
		Update("enable", false)
	err := result.Error
	count := result.RowsAffected
	return count, err
}
