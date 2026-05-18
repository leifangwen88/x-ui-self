package service

import (
	"time"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/util/common"

	"gorm.io/gorm"
)

type SocksGameService struct{}

func (s *SocksGameService) socksAddrPort(socksProxyId int) (string, int) {
	if socksProxyId <= 0 {
		return "", 0
	}
	db := database.GetDB()
	sp := &model.SocksProxy{}
	if err := db.First(sp, socksProxyId).Error; err != nil {
		return "", 0
	}
	return sp.Address, sp.Port
}

func (s *SocksGameService) fillSocksMeta(st *model.SocksGameStatus, socksProxyId int) {
	if st == nil || socksProxyId <= 0 {
		return
	}
	addr, port := s.socksAddrPort(socksProxyId)
	if addr != "" && port > 0 {
		st.SocksAddress = addr
		st.SocksPort = port
	}
}

func (s *SocksGameService) findStatus(socksProxyId, gameId int) (*model.SocksGameStatus, error) {
	if gameId <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	db := database.GetDB()
	addr, port := s.socksAddrPort(socksProxyId)
	st := &model.SocksGameStatus{}
	var err error
	if addr != "" && port > 0 {
		err = db.Where("game_id = ? AND socks_address = ? AND socks_port = ?", gameId, addr, port).First(st).Error
	} else if socksProxyId > 0 {
		err = db.Where("socks_proxy_id = ? AND game_id = ?", socksProxyId, gameId).First(st).Error
	} else {
		return nil, gorm.ErrRecordNotFound
	}
	if err != nil {
		return nil, err
	}
	return st, nil
}

func (s *SocksGameService) emitMarkSync(socksProxyId, gameId int, mark, note string) {
	if globalPanelSync == nil || globalPanelSync.IsApplying() {
		return
	}
	globalPanelSync.EmitGameMark(socksProxyId, gameId, mark, note)
}

func (s *SocksGameService) GetAllStatuses() ([]*model.SocksGameStatus, error) {
	db := database.GetDB()
	var list []*model.SocksGameStatus
	err := db.Find(&list).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	return list, nil
}

func (s *SocksGameService) GetStatus(socksProxyId, gameId int) (*model.SocksGameStatus, error) {
	return s.findStatus(socksProxyId, gameId)
}

func (s *SocksGameService) IsBanned(socksProxyId, gameId int) bool {
	if gameId <= 0 {
		return false
	}
	st, err := s.findStatus(socksProxyId, gameId)
	if err != nil {
		return false
	}
	return st.Status == model.SocksGameStatusBanned
}

func (s *SocksGameService) IsUsed(socksProxyId, gameId int) bool {
	if gameId <= 0 {
		return false
	}
	st, err := s.findStatus(socksProxyId, gameId)
	if err != nil {
		return false
	}
	if st.Status == model.SocksGameStatusBanned {
		return false
	}
	return st.UseCount > 0 || st.Status == model.SocksGameStatusUsed
}

func (s *SocksGameService) MarkUsed(socksProxyId, gameId int, note string) error {
	if socksProxyId <= 0 || gameId <= 0 {
		return common.NewError("无效的 SOCKS 或游戏")
	}
	if s.IsBanned(socksProxyId, gameId) {
		return common.NewError("该 IP 在此游戏已封禁，无法仅标记为用过")
	}
	db := database.GetDB()
	now := time.Now().UnixMilli()
	st, err := s.findStatus(socksProxyId, gameId)
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	if err == gorm.ErrRecordNotFound {
		st = &model.SocksGameStatus{
			SocksProxyId: socksProxyId,
			GameId:       gameId,
			Status:       model.SocksGameStatusUsed,
			LastUsedAt:   now,
			UseCount:     1,
			Note:         note,
		}
		s.fillSocksMeta(st, socksProxyId)
		if err := db.Create(st).Error; err != nil {
			return err
		}
		s.emitMarkSync(socksProxyId, gameId, model.SocksGameMarkUsed, note)
		return nil
	}
	st.Status = model.SocksGameStatusUsed
	st.LastUsedAt = now
	if st.UseCount < 1 {
		st.UseCount = 1
	}
	if note != "" {
		st.Note = note
	}
	s.fillSocksMeta(st, socksProxyId)
	if err := db.Save(st).Error; err != nil {
		return err
	}
	s.emitMarkSync(socksProxyId, gameId, model.SocksGameMarkUsed, note)
	return nil
}

func (s *SocksGameService) MarkBanned(socksProxyId, gameId int, note string) error {
	if socksProxyId <= 0 || gameId <= 0 {
		return common.NewError("无效的 SOCKS 或游戏")
	}
	db := database.GetDB()
	now := time.Now().UnixMilli()
	st, err := s.findStatus(socksProxyId, gameId)
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	if err == gorm.ErrRecordNotFound {
		st = &model.SocksGameStatus{
			SocksProxyId: socksProxyId,
			GameId:       gameId,
			Status:       model.SocksGameStatusBanned,
			BannedAt:     now,
			UseCount:     0,
			Note:         note,
		}
		s.fillSocksMeta(st, socksProxyId)
		if err := db.Create(st).Error; err != nil {
			return err
		}
		s.emitMarkSync(socksProxyId, gameId, model.SocksGameMarkBanned, note)
		return nil
	}
	st.Status = model.SocksGameStatusBanned
	st.BannedAt = now
	if note != "" {
		st.Note = note
	}
	s.fillSocksMeta(st, socksProxyId)
	if err := db.Save(st).Error; err != nil {
		return err
	}
	s.emitMarkSync(socksProxyId, gameId, model.SocksGameMarkBanned, note)
	return nil
}

func (s *SocksGameService) ClearMark(socksProxyId, gameId int) error {
	if socksProxyId <= 0 || gameId <= 0 {
		return common.NewError("无效的 SOCKS 或游戏")
	}
	addr, port := s.socksAddrPort(socksProxyId)
	db := database.GetDB()
	q := db.Where("game_id = ?", gameId)
	if addr != "" && port > 0 {
		q = q.Where("socks_address = ? AND socks_port = ?", addr, port)
	} else {
		q = q.Where("socks_proxy_id = ?", socksProxyId)
	}
	if err := q.Delete(&model.SocksGameStatus{}).Error; err != nil {
		return err
	}
	s.emitMarkSync(socksProxyId, gameId, "clear", "")
	return nil
}

func (s *SocksGameService) SetMark(socksProxyId, gameId int, mark string, note string) error {
	switch mark {
	case model.SocksGameMarkUsed:
		return s.MarkUsed(socksProxyId, gameId, note)
	case model.SocksGameMarkBanned:
		return s.MarkBanned(socksProxyId, gameId, note)
	case "clear":
		return s.ClearMark(socksProxyId, gameId)
	default:
		return common.NewError("无效标记，请使用 used、banned 或 clear")
	}
}

func (s *SocksGameService) SetBanned(socksProxyId, gameId int, banned bool, note string) error {
	if banned {
		return s.MarkBanned(socksProxyId, gameId, note)
	}
	if socksProxyId <= 0 || gameId <= 0 {
		return common.NewError("无效的 SOCKS 或游戏")
	}
	st, err := s.GetStatus(socksProxyId, gameId)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}
	if st.Status != model.SocksGameStatusBanned {
		return nil
	}
	if st.UseCount > 0 {
		st.Status = model.SocksGameStatusUsed
	} else {
		st.Status = model.SocksGameStatusActive
	}
	st.BannedAt = 0
	if note != "" {
		st.Note = note
	}
	if err := database.GetDB().Save(st).Error; err != nil {
		return err
	}
	EmitMarkUnban(socksProxyId, gameId)
	return nil
}

func (s *SocksGameService) RecordUsage(socksProxyId, gameId int) error {
	if s.IsBanned(socksProxyId, gameId) {
		return nil
	}
	return s.MarkUsed(socksProxyId, gameId, "")
}

func (s *SocksGameService) GameUseCount(socksProxyId, gameId int) int {
	st, err := s.GetStatus(socksProxyId, gameId)
	if err != nil {
		return 0
	}
	return st.UseCount
}
