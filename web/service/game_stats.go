package service

import "x-ui/database/model"

type GameIpStats struct {
	GameId         int `json:"gameId"`
	FreshAvailable int `json:"freshAvailable"`
	OldAvailable   int `json:"oldAvailable"`
	BannedCount    int `json:"bannedCount"`
}

type GameBoundInbound struct {
	Id           int    `json:"id"`
	Remark       string `json:"remark"`
	Port         int    `json:"port"`
	Protocol     string `json:"protocol"`
	Enable       bool   `json:"enable"`
	SocksProxyId int    `json:"socksProxyId"`
}

type GameWithStats struct {
	*model.Game
	Stats    GameIpStats          `json:"stats"`
	Inbounds []GameBoundInbound   `json:"inbounds"`
}

func (s *GameService) ListWithStats() ([]*GameWithStats, error) {
	games, err := s.GetAll()
	if err != nil {
		return nil, err
	}
	socksService := SocksProxyService{}
	socksList, err := socksService.GetAll()
	if err != nil {
		return nil, err
	}
	socksGame := SocksGameService{}
	statuses, err := socksGame.GetAllStatuses()
	if err != nil {
		return nil, err
	}
	inboundService := InboundService{}
	allInbounds, err := inboundService.GetAllInbounds()
	if err != nil {
		return nil, err
	}
	inboundsByGame := make(map[int][]GameBoundInbound)
	for _, ib := range allInbounds {
		if ib.GameId <= 0 {
			continue
		}
		inboundsByGame[ib.GameId] = append(inboundsByGame[ib.GameId], GameBoundInbound{
			Id:           ib.Id,
			Remark:       ib.Remark,
			Port:         ib.Port,
			Protocol:     string(ib.Protocol),
			Enable:       ib.Enable,
			SocksProxyId: ib.SocksProxyId,
		})
	}

	statusMap := make(map[int]map[int]*model.SocksGameStatus)
	bannedPerGame := make(map[int]int)
	for _, st := range statuses {
		if statusMap[st.SocksProxyId] == nil {
			statusMap[st.SocksProxyId] = make(map[int]*model.SocksGameStatus)
		}
		statusMap[st.SocksProxyId][st.GameId] = st
		if st.Status == model.SocksGameStatusBanned {
			bannedPerGame[st.GameId]++
		}
	}

	result := make([]*GameWithStats, 0, len(games))
	for _, game := range games {
		stats := GameIpStats{
			GameId:      game.Id,
			BannedCount: bannedPerGame[game.Id],
		}
		for _, socks := range socksList {
			if !socks.Enable {
				continue
			}
			st := statusMap[socks.Id][game.Id]
			if st != nil && st.Status == model.SocksGameStatusBanned {
				continue
			}
			if socksService.IsExpired(socks) {
				continue
			}
			used := st != nil && (st.UseCount > 0 || st.Status == model.SocksGameStatusUsed)
			if used {
				stats.OldAvailable++
			} else {
				stats.FreshAvailable++
			}
		}
		bound := inboundsByGame[game.Id]
		if bound == nil {
			bound = []GameBoundInbound{}
		}
		result = append(result, &GameWithStats{
			Game:     game,
			Stats:    stats,
			Inbounds: bound,
		})
	}
	return result, nil
}
