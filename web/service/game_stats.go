package service

import "x-ui/database/model"

type GameIpStats struct {
	GameId         int `json:"gameId"`
	FreshAvailable int `json:"freshAvailable"`
	OldAvailable   int `json:"oldAvailable"`
	BannedCount    int `json:"bannedCount"`
}

type GameWithStats struct {
	*model.Game
	Stats GameIpStats `json:"stats"`
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
	statusMap := make(map[int]map[int]*model.SocksGameStatus)
	for _, st := range statuses {
		if statusMap[st.SocksProxyId] == nil {
			statusMap[st.SocksProxyId] = make(map[int]*model.SocksGameStatus)
		}
		statusMap[st.SocksProxyId][st.GameId] = st
	}

	result := make([]*GameWithStats, 0, len(games))
	for _, game := range games {
		stats := GameIpStats{GameId: game.Id}
		for _, socks := range socksList {
			if !socks.Enable {
				continue
			}
			st := statusMap[socks.Id][game.Id]
			if st != nil && st.Status == model.SocksGameStatusBanned {
				stats.BannedCount++
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
		result = append(result, &GameWithStats{
			Game:  game,
			Stats: stats,
		})
	}
	return result, nil
}
