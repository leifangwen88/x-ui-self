package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/util/common"

	"gorm.io/gorm"
)

const (
	SyncEventGameUpsert       = "game_upsert"
	SyncEventGameDelete       = "game_delete"
	SyncEventSocksUpsert      = "socks_upsert"
	SyncEventInboundUpsert    = "inbound_upsert"
	SyncEventInboundDelete    = "inbound_delete"
	SyncEventMarkUnban        = "game_mark_unban"
	SyncEventXrayTemplate     = "setting_xray_template"
)

type syncGameDeletePayload struct {
	Code string `json:"code"`
}

type syncXrayTemplatePayload struct {
	Config string `json:"config"`
}

func BuildGameBackup(g *model.Game) GameBackup {
	if g == nil {
		return GameBackup{}
	}
	code := strings.TrimSpace(g.Code)
	if code == "" {
		code = fmt.Sprintf("game_%d", g.Id)
	}
	return GameBackup{
		Name:      g.Name,
		Code:      code,
		Enable:    g.Enable,
		SortOrder: g.SortOrder,
		Remark:    g.Remark,
	}
}

func BuildSocksBackup(sp *model.SocksProxy) SocksBackup {
	if sp == nil {
		return SocksBackup{}
	}
	return SocksBackup{
		Address:    sp.Address,
		Port:       sp.Port,
		Username:   sp.Username,
		Password:   sp.Password,
		Enable:     sp.Enable,
		Remark:     sp.Remark,
		CreatedAt:  sp.CreatedAt,
		ExpiryTime: sp.ExpiryTime,
	}
}

func BuildInboundBackup(ib *model.Inbound) (InboundBackup, error) {
	if ib == nil {
		return InboundBackup{}, common.NewError("入站为空")
	}
	item := InboundBackup{
		Port:           ib.Port,
		Remark:         ib.Remark,
		Enable:         ib.Enable,
		ExpiryTime:     ib.ExpiryTime,
		Listen:         ib.Listen,
		Protocol:       string(ib.Protocol),
		Settings:       ib.Settings,
		StreamSettings: ib.StreamSettings,
		Sniffing:       ib.Sniffing,
		RotationEnable: ib.RotationEnable,
		RotationPolicy: ib.RotationPolicy,
		LastRotatedAt:  ib.LastRotatedAt,
	}
	if item.RotationPolicy == "" {
		item.RotationPolicy = model.RotationPolicyPreferUnusedUnbanned
	}
	db := database.GetDB()
	if ib.SocksProxyId > 0 {
		sp := &model.SocksProxy{}
		if err := db.First(sp, ib.SocksProxyId).Error; err == nil {
			item.SocksKey = SocksNaturalKey(sp.Address, sp.Port)
		}
	}
	if ib.GameId > 0 {
		g := &model.Game{}
		if err := db.First(g, ib.GameId).Error; err == nil {
			code := strings.TrimSpace(g.Code)
			if code == "" {
				code = fmt.Sprintf("game_%d", g.Id)
			}
			item.GameCode = code
		}
	}
	return item, nil
}

func EmitGameUpsert(game *model.Game) {
	if game == nil {
		return
	}
	syncEmit(SyncEventGameUpsert, BuildGameBackup(game))
}

func EmitGameDeleteByCode(code string) {
	code = strings.TrimSpace(code)
	if code == "" {
		return
	}
	syncEmit(SyncEventGameDelete, syncGameDeletePayload{Code: code})
}

func EmitSocksUpsert(sp *model.SocksProxy) {
	if sp == nil {
		return
	}
	syncEmit(SyncEventSocksUpsert, BuildSocksBackup(sp))
}

func EmitInboundUpsertById(id int) {
	if id <= 0 {
		return
	}
	inboundSvc := InboundService{}
	ib, err := inboundSvc.GetInbound(id)
	if err != nil {
		return
	}
	EmitInboundUpsert(ib)
}

func EmitInboundUpsert(ib *model.Inbound) {
	if ib == nil {
		return
	}
	item, err := BuildInboundBackup(ib)
	if err != nil {
		return
	}
	syncEmit(SyncEventInboundUpsert, item)
}

func EmitInboundDeletePort(port int) {
	if port <= 0 {
		return
	}
	syncEmit(SyncEventInboundDelete, syncInboundSocksPayload{InboundPort: port})
}

func EmitMarkUnban(socksProxyId, gameId int) {
	if globalPanelSync == nil || globalPanelSync.IsApplying() {
		return
	}
	db := database.GetDB()
	sp := &model.SocksProxy{}
	if socksProxyId <= 0 || gameId <= 0 {
		return
	}
	if err := db.First(sp, socksProxyId).Error; err != nil {
		return
	}
	g := &model.Game{}
	if err := db.First(g, gameId).Error; err != nil {
		return
	}
	code := strings.TrimSpace(g.Code)
	if code == "" {
		return
	}
	syncEmit(SyncEventMarkUnban, syncMarkPayload{
		SocksKey: SocksNaturalKey(sp.Address, sp.Port),
		GameCode: code,
	})
}

func EmitXrayTemplate(config string) {
	syncEmit(SyncEventXrayTemplate, syncXrayTemplatePayload{Config: config})
}

func (s *PanelSyncService) applyGameUpsert(raw json.RawMessage) error {
	var g GameBackup
	if err := json.Unmarshal(raw, &g); err != nil {
		return err
	}
	code := strings.TrimSpace(g.Code)
	if code == "" {
		return common.NewError("game code 为空")
	}
	db := database.GetDB()
	row := &model.Game{}
	err := db.Where("code = ?", code).First(row).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	if err == gorm.ErrRecordNotFound {
		row = &model.Game{
			Name:      g.Name,
			Code:      code,
			Enable:    g.Enable,
			SortOrder: g.SortOrder,
			Remark:    g.Remark,
		}
		return db.Create(row).Error
	}
	row.Name = g.Name
	row.Enable = g.Enable
	row.SortOrder = g.SortOrder
	row.Remark = g.Remark
	return db.Save(row).Error
}

func (s *PanelSyncService) applyGameDelete(raw json.RawMessage) error {
	var p syncGameDeletePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	code := strings.TrimSpace(p.Code)
	if code == "" {
		return nil
	}
	db := database.GetDB()
	g := &model.Game{}
	if err := db.Where("code = ?", code).First(g).Error; err != nil {
		return nil
	}
	var inboundCount int64
	db.Model(model.Inbound{}).Where("game_id = ?", g.Id).Count(&inboundCount)
	if inboundCount > 0 {
		return nil
	}
	return db.Delete(g).Error
}

func (s *PanelSyncService) applySocksUpsert(raw json.RawMessage) error {
	var sp SocksBackup
	if err := json.Unmarshal(raw, &sp); err != nil {
		return err
	}
	key := SocksNaturalKey(sp.Address, sp.Port)
	if key == ":0" {
		return nil
	}
	db := database.GetDB()
	row := &model.SocksProxy{}
	err := db.Where("address = ? AND port = ?", strings.TrimSpace(sp.Address), sp.Port).First(row).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	if err == gorm.ErrRecordNotFound {
		row = &model.SocksProxy{
			Address:    strings.TrimSpace(sp.Address),
			Port:       sp.Port,
			Username:   sp.Username,
			Password:   sp.Password,
			Enable:     sp.Enable,
			Remark:     sp.Remark,
			CreatedAt:  sp.CreatedAt,
			ExpiryTime: sp.ExpiryTime,
		}
		if row.CreatedAt <= 0 {
			row.CreatedAt = time.Now().UnixMilli()
		}
		return db.Create(row).Error
	}
	row.Username = sp.Username
	row.Password = sp.Password
	row.Enable = sp.Enable
	row.Remark = sp.Remark
	row.ExpiryTime = sp.ExpiryTime
	return db.Save(row).Error
}

func (s *PanelSyncService) applyInboundUpsert(raw json.RawMessage) error {
	var ib InboundBackup
	if err := json.Unmarshal(raw, &ib); err != nil {
		return err
	}
	if ib.Port <= 0 || ib.Port > 65535 {
		return nil
	}
	userId, err := primaryUserID()
	if err != nil {
		return err
	}
	db := database.GetDB()
	exist := &model.Inbound{}
	err = db.Where("port = ?", ib.Port).First(exist).Error
	policy := ib.RotationPolicy
	if policy == "" {
		policy = model.RotationPolicyPreferUnusedUnbanned
	}
	socksID := 0
	if strings.TrimSpace(ib.SocksKey) != "" {
		id, _, _, ok := s.resolveSocks(ib.SocksKey)
		if !ok {
			return nil
		}
		socksID = id
	}
	gameID := 0
	if strings.TrimSpace(ib.GameCode) != "" {
		id, err := s.gameIDByCode(ib.GameCode)
		if err != nil {
			return nil
		}
		gameID = id
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	if err == gorm.ErrRecordNotFound {
		row := &model.Inbound{
			UserId:         userId,
			Remark:         ib.Remark,
			Enable:         ib.Enable,
			ExpiryTime:     ib.ExpiryTime,
			Listen:         ib.Listen,
			Port:           ib.Port,
			Protocol:       model.Protocol(ib.Protocol),
			Settings:       ib.Settings,
			StreamSettings: ib.StreamSettings,
			Sniffing:       ib.Sniffing,
			Tag:            fmt.Sprintf("inbound-%v", ib.Port),
			SocksProxyId:   socksID,
			GameId:         gameID,
			RotationEnable: ib.RotationEnable,
			RotationPolicy: policy,
			LastRotatedAt:  ib.LastRotatedAt,
		}
		if err := db.Create(row).Error; err != nil {
			return err
		}
		s.xrayService.SetToNeedRestart()
		return nil
	}
	exist.Remark = ib.Remark
	exist.Enable = ib.Enable
	exist.ExpiryTime = ib.ExpiryTime
	exist.Listen = ib.Listen
	exist.Protocol = model.Protocol(ib.Protocol)
	exist.Settings = ib.Settings
	exist.StreamSettings = ib.StreamSettings
	exist.Sniffing = ib.Sniffing
	exist.SocksProxyId = socksID
	exist.GameId = gameID
	exist.RotationEnable = ib.RotationEnable
	exist.RotationPolicy = policy
	exist.LastRotatedAt = ib.LastRotatedAt
	exist.Tag = fmt.Sprintf("inbound-%v", ib.Port)
	if err := db.Save(exist).Error; err != nil {
		return err
	}
	s.xrayService.SetToNeedRestart()
	return nil
}

func (s *PanelSyncService) applyInboundDelete(raw json.RawMessage) error {
	var p syncInboundSocksPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	if p.InboundPort <= 0 {
		return nil
	}
	db := database.GetDB()
	res := db.Where("port = ?", p.InboundPort).Delete(&model.Inbound{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		s.xrayService.SetToNeedRestart()
	}
	return nil
}

func (s *PanelSyncService) applyMarkUnban(raw json.RawMessage) error {
	var p syncMarkPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	gameID, err := s.gameIDByCode(p.GameCode)
	if err != nil {
		return err
	}
	socksID, _, _, ok := s.resolveSocks(p.SocksKey)
	if !ok || socksID <= 0 {
		return nil
	}
	sg := SocksGameService{}
	return sg.SetBanned(socksID, gameID, false, p.Note)
}

func (s *PanelSyncService) applyXrayTemplate(raw json.RawMessage) error {
	var p syncXrayTemplatePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	return s.settingService.setString("xrayTemplateConfig", p.Config)
}
