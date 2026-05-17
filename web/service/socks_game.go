package service

import (
	"time"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/util/common"

	"gorm.io/gorm"
)

type SocksGameService struct{}

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
	db := database.GetDB()
	st := &model.SocksGameStatus{}
	err := db.Where("socks_proxy_id = ? AND game_id = ?", socksProxyId, gameId).First(st).Error
	if err != nil {
		return nil, err
	}
	return st, nil
}

func (s *SocksGameService) IsBanned(socksProxyId, gameId int) bool {
	if socksProxyId <= 0 || gameId <= 0 {
		return false
	}
	st, err := s.GetStatus(socksProxyId, gameId)
	if err != nil {
		return false
	}
	return st.Status == model.SocksGameStatusBanned
}

func (s *SocksGameService) IsUsed(socksProxyId, gameId int) bool {
	if socksProxyId <= 0 || gameId <= 0 {
		return false
	}
	st, err := s.GetStatus(socksProxyId, gameId)
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
	db := database.GetDB()
	now := time.Now().UnixMilli()
	st := &model.SocksGameStatus{}
	err := db.Where("socks_proxy_id = ? AND game_id = ?", socksProxyId, gameId).First(st).Error
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
		return db.Create(st).Error
	}
	if st.Status == model.SocksGameStatusBanned {
		return common.NewError("该 IP 在此游戏已封禁，无法仅标记为用过")
	}
	st.Status = model.SocksGameStatusUsed
	st.LastUsedAt = now
	if st.UseCount < 1 {
		st.UseCount = 1
	}
	if note != "" {
		st.Note = note
	}
	return db.Save(st).Error
}

func (s *SocksGameService) MarkBanned(socksProxyId, gameId int, note string) error {
	if socksProxyId <= 0 || gameId <= 0 {
		return common.NewError("无效的 SOCKS 或游戏")
	}
	db := database.GetDB()
	now := time.Now().UnixMilli()
	st := &model.SocksGameStatus{}
	err := db.Where("socks_proxy_id = ? AND game_id = ?", socksProxyId, gameId).First(st).Error
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
		return db.Create(st).Error
	}
	st.Status = model.SocksGameStatusBanned
	st.BannedAt = now
	if note != "" {
		st.Note = note
	}
	return db.Save(st).Error
}

func (s *SocksGameService) ClearMark(socksProxyId, gameId int) error {
	if socksProxyId <= 0 || gameId <= 0 {
		return common.NewError("无效的 SOCKS 或游戏")
	}
	return database.GetDB().Where("socks_proxy_id = ? AND game_id = ?", socksProxyId, gameId).
		Delete(&model.SocksGameStatus{}).Error
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
	return database.GetDB().Save(st).Error
}

func (s *SocksGameService) RecordUsage(socksProxyId, gameId int) error {
	return s.MarkUsed(socksProxyId, gameId, "")
}

func (s *SocksGameService) GameUseCount(socksProxyId, gameId int) int {
	st, err := s.GetStatus(socksProxyId, gameId)
	if err != nil {
		return 0
	}
	return st.UseCount
}
