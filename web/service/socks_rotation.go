package service

import (
	"fmt"
	"time"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/util/common"
)

type SocksRotationService struct {
	inboundService    InboundService
	socksProxyService SocksProxyService
	socksGameService  SocksGameService
}

type RotateResult struct {
	FromSocksId int `json:"fromSocksId"`
	ToSocksId   int `json:"toSocksId"`
	GameId      int `json:"gameId"`
}

func (s *SocksRotationService) buildSocksBoundMap(inbounds []*model.Inbound) map[int]int {
	m := make(map[int]int)
	for _, ib := range inbounds {
		if ib.SocksProxyId > 0 {
			m[ib.SocksProxyId] = ib.Id
		}
	}
	return m
}

func (s *SocksRotationService) scoreSocksForGame(socks *model.SocksProxy, gameId int, excludeSocksId int) (int, bool) {
	if excludeSocksId > 0 && socks.Id == excludeSocksId {
		return 0, false
	}
	if !socks.Enable || s.socksProxyService.IsExpired(socks) {
		return 0, false
	}
	if s.socksGameService.IsBanned(socks.Id, gameId) {
		return 0, false
	}
	useCount := s.socksGameService.GameUseCount(socks.Id, gameId)
	score := 0
	if useCount == 0 {
		score += 1000000
	} else {
		st, err := s.socksGameService.GetStatus(socks.Id, gameId)
		if err == nil && st.LastUsedAt > 0 {
			score += int(time.Now().UnixMilli()-st.LastUsedAt) / 1000
		}
	}
	return score, true
}

func (s *SocksRotationService) PickForInbound(inbound *model.Inbound, excludeSocksId int) (*model.SocksProxy, error) {
	if inbound.GameId <= 0 {
		return nil, common.NewError("请先为入站绑定游戏")
	}
	gameService := &GameService{}
	_, err := gameService.GetById(inbound.GameId)
	if err != nil {
		return nil, common.NewError("入站绑定的游戏不存在")
	}

	socksList, err := s.socksProxyService.GetAll()
	if err != nil {
		return nil, err
	}
	inbounds, err := s.inboundService.GetAllInbounds()
	if err != nil {
		return nil, err
	}
	boundMap := s.buildSocksBoundMap(inbounds)

	var best *model.SocksProxy
	bestScore := -1
	for _, socks := range socksList {
		owner := boundMap[socks.Id]
		if owner > 0 && owner != inbound.Id {
			continue
		}
		score, ok := s.scoreSocksForGame(socks, inbound.GameId, excludeSocksId)
		if !ok {
			continue
		}
		if score > bestScore {
			bestScore = score
			best = socks
		}
	}
	if best == nil {
		return nil, common.NewError("没有可用的 SOCKS（需未到期、未被他站绑定、且在该游戏未封禁；用过也可轮换）")
	}
	return best, nil
}

func (s *SocksRotationService) appendLog(inboundId, gameId, fromId, toId int, reason string) error {
	db := database.GetDB()
	return db.Create(&model.SocksRotationLog{
		InboundId:   inboundId,
		GameId:      gameId,
		FromSocksId: fromId,
		ToSocksId:   toId,
		Reason:      reason,
		CreatedAt:   time.Now().UnixMilli(),
	}).Error
}

func (s *SocksRotationService) applyOutgoingMark(socksId, gameId int, mark string) error {
	switch mark {
	case model.SocksGameMarkUsed:
		return s.socksGameService.MarkUsed(socksId, gameId, "轮换标记用过")
	case model.SocksGameMarkBanned:
		return s.socksGameService.MarkBanned(socksId, gameId, "轮换标记封禁")
	default:
		return common.NewError("请先标记当前 IP：用过 或 封禁过")
	}
}

func (s *SocksRotationService) RotateInbound(inboundId int, outgoingMark string, reason string) (*RotateResult, error) {
	inbound, err := s.inboundService.GetInbound(inboundId)
	if err != nil {
		return nil, err
	}
	if inbound.GameId <= 0 {
		return nil, common.NewError("请先为入站绑定游戏")
	}
	fromId := inbound.SocksProxyId
	if fromId > 0 {
		if outgoingMark != model.SocksGameMarkUsed && outgoingMark != model.SocksGameMarkBanned {
			return nil, common.NewError("轮换前必须标记当前 IP：用过 或 封禁过")
		}
		if err := s.applyOutgoingMark(fromId, inbound.GameId, outgoingMark); err != nil {
			return nil, err
		}
	}
	pick, err := s.PickForInbound(inbound, fromId)
	if err != nil {
		return nil, err
	}
	if reason == "" {
		reason = "manual"
		if outgoingMark == model.SocksGameMarkBanned {
			reason = "banned"
		} else if outgoingMark == model.SocksGameMarkUsed {
			reason = "used"
		}
	}
	if err := s.inboundService.UpdateSocksProxyId(inboundId, pick.Id); err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	db := database.GetDB()
	_ = db.Model(model.Inbound{}).Where("id = ?", inboundId).Update("last_rotated_at", now).Error
	_ = s.appendLog(inboundId, inbound.GameId, fromId, pick.Id, reason)
	return &RotateResult{
		FromSocksId: fromId,
		ToSocksId:   pick.Id,
		GameId:      inbound.GameId,
	}, nil
}

type GameRotateCheckResult struct {
	GameId         int    `json:"gameId"`
	InboundCount   int    `json:"inboundCount"`
	PoolAvailable  int    `json:"poolAvailable"`
	Enough         bool   `json:"enough"`
	Message        string `json:"message"`
}

type GameBatchRotateItem struct {
	InboundId    int    `json:"inboundId" form:"inboundId"`
	OutgoingMark string `json:"outgoingMark" form:"outgoingMark"`
}

func (s *SocksRotationService) CheckGameRotate(gameId int) (*GameRotateCheckResult, error) {
	if gameId <= 0 {
		return nil, common.NewError("无效的游戏")
	}
	gameService := GameService{}
	list, err := gameService.ListWithStats()
	if err != nil {
		return nil, err
	}
	var stats *GameIpStats
	for _, g := range list {
		if g.Id == gameId {
			stats = &g.Stats
			break
		}
	}
	if stats == nil {
		return nil, common.NewError("游戏不存在")
	}
	inbounds, err := s.inboundService.GetAllInbounds()
	if err != nil {
		return nil, err
	}
	count := 0
	for _, ib := range inbounds {
		if ib.GameId == gameId && ib.Enable {
			count++
		}
	}
	pool := stats.FreshAvailable + stats.OldAvailable
	res := &GameRotateCheckResult{
		GameId:        gameId,
		InboundCount:  count,
		PoolAvailable: pool,
		Enough:        pool >= count && count > 0,
	}
	if count == 0 {
		res.Message = "该游戏下没有已启用的入站"
	} else if !res.Enough {
		res.Message = fmt.Sprintf("可用 IP 不足：需要 %d 个，当前可用 %d 个（未过期且未封禁）", count, pool)
	} else {
		res.Message = fmt.Sprintf("可轮换 %d 个入站，当前可用 IP %d 个", count, pool)
	}
	return res, nil
}

func (s *SocksRotationService) BatchRotateGame(gameId int, items []GameBatchRotateItem) (int, error) {
	if len(items) == 0 {
		return 0, common.NewError("没有可轮换的入站")
	}
	check, err := s.CheckGameRotate(gameId)
	if err != nil {
		return 0, err
	}
	if !check.Enough {
		return 0, common.NewError(check.Message)
	}
	ok := 0
	for _, item := range items {
		ib, err := s.inboundService.GetInbound(item.InboundId)
		if err != nil {
			return ok, err
		}
		if ib.GameId != gameId {
			return ok, common.NewError("入站与游戏不匹配:", item.InboundId)
		}
		_, err = s.RotateInbound(item.InboundId, item.OutgoingMark, "game_batch")
		if err != nil {
			return ok, err
		}
		ok++
	}
	return ok, nil
}
