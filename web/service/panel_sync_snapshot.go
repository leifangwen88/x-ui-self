package service

import (
	"fmt"
	"strings"
	"time"
	"x-ui/database"
	"x-ui/database/model"
)

func (s *PanelSyncService) BuildLocalSnapshot() (*PanelSyncSnapshot, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	db := database.GetDB()

	var games []*model.Game
	if err := db.Order("sort_order asc, id asc").Find(&games).Error; err != nil {
		return nil, err
	}
	gameCodeByID := make(map[int]string, len(games))
	gameExports := make([]GameBackup, 0, len(games))
	for _, g := range games {
		code := strings.TrimSpace(g.Code)
		if code == "" {
			code = fmt.Sprintf("game_%d", g.Id)
		}
		gameCodeByID[g.Id] = code
		gameExports = append(gameExports, GameBackup{
			Name:      g.Name,
			Code:      code,
			Enable:    g.Enable,
			SortOrder: g.SortOrder,
			Remark:    g.Remark,
		})
	}

	var socksList []*model.SocksProxy
	if err := db.Order("id asc").Find(&socksList).Error; err != nil {
		return nil, err
	}
	socksKeyByID := make(map[int]string, len(socksList))
	socksExports := make([]SocksBackup, 0, len(socksList))
	for _, sp := range socksList {
		key := SocksNaturalKey(sp.Address, sp.Port)
		socksKeyByID[sp.Id] = key
		socksExports = append(socksExports, SocksBackup{
			Address:    sp.Address,
			Port:       sp.Port,
			Username:   sp.Username,
			Password:   sp.Password,
			Enable:     sp.Enable,
			Remark:     sp.Remark,
			CreatedAt:  sp.CreatedAt,
			ExpiryTime: sp.ExpiryTime,
		})
	}

	var inbounds []*model.Inbound
	if err := db.Order("id asc").Find(&inbounds).Error; err != nil {
		return nil, err
	}
	inboundExports := make([]InboundBackup, 0, len(inbounds))
	for _, ib := range inbounds {
		item := InboundBackup{
			Port:           ib.Port,
			Remark:         ib.Remark,
			Enable:         ib.Enable,
			ExpiryTime:     ib.ExpiryTime,
			Up:             ib.Up,
			Down:           ib.Down,
			Total:          ib.Total,
			Listen:         ib.Listen,
			Protocol:       string(ib.Protocol),
			Settings:       ib.Settings,
			StreamSettings: ib.StreamSettings,
			Sniffing:       ib.Sniffing,
			RotationEnable: ib.RotationEnable,
			RotationPolicy: ib.RotationPolicy,
			LastRotatedAt:  ib.LastRotatedAt,
		}
		if ib.SocksProxyId > 0 {
			item.SocksKey = socksKeyByID[ib.SocksProxyId]
		}
		if ib.GameId > 0 {
			item.GameCode = gameCodeByID[ib.GameId]
		}
		inboundExports = append(inboundExports, item)
	}

	socksGame := SocksGameService{}
	statuses, err := socksGame.GetAllStatuses()
	if err != nil {
		return nil, err
	}
	markExports := make([]SocksGameMarkBackup, 0, len(statuses))
	for _, st := range statuses {
		sk := socksKeyByID[st.SocksProxyId]
		if sk == "" && strings.TrimSpace(st.SocksAddress) != "" {
			sk = SocksNaturalKey(st.SocksAddress, st.SocksPort)
		}
		gc := gameCodeByID[st.GameId]
		if sk == "" || gc == "" {
			continue
		}
		markExports = append(markExports, SocksGameMarkBackup{
			SocksKey:   sk,
			GameCode:   gc,
			Status:     st.Status,
			BannedAt:   st.BannedAt,
			LastUsedAt: st.LastUsedAt,
			UseCount:   st.UseCount,
			Note:       st.Note,
		})
	}

	xrayTpl, _ := s.settingService.GetXrayConfigTemplate()
	return &PanelSyncSnapshot{
		NodeId:             cfg.NodeId,
		ExportedAt:         time.Now().UnixMilli(),
		Games:              gameExports,
		SocksProxies:       socksExports,
		Inbounds:           inboundExports,
		SocksGameMarks:     markExports,
		XrayTemplateConfig: xrayTpl,
	}, nil
}

func snapshotCounts(s *PanelSyncSnapshot) PanelAlignCounts {
	if s == nil {
		return PanelAlignCounts{}
	}
	xt := 0
	if strings.TrimSpace(s.XrayTemplateConfig) != "" {
		xt = 1
	}
	return PanelAlignCounts{
		Games:        len(s.Games),
		Socks:        len(s.SocksProxies),
		Inbounds:     len(s.Inbounds),
		Marks:        len(s.SocksGameMarks),
		XrayTemplate: xt,
	}
}

func compareSnapshots(local, peer *PanelSyncSnapshot) PanelAlignDiff {
	diff := PanelAlignDiff{}
	if local == nil || peer == nil {
		return diff
	}
	diff.Games = diffGames(local.Games, peer.Games)
	diff.Socks = diffSocks(local.SocksProxies, peer.SocksProxies)
	diff.Inbounds = diffInbounds(local.Inbounds, peer.Inbounds)
	diff.InboundRemarks = diffInboundRemarks(local.Inbounds, peer.Inbounds)
	diff.Marks = diffMarks(local.SocksGameMarks, peer.SocksGameMarks)
	diff.XrayTemplate = diffXrayTemplate(local.XrayTemplateConfig, peer.XrayTemplateConfig)
	return diff
}

func diffXrayTemplate(local, peer string) PanelAlignCategoryDiff {
	d := PanelAlignCategoryDiff{}
	if strings.TrimSpace(local) == strings.TrimSpace(peer) {
		return d
	}
	if strings.TrimSpace(local) == "" {
		d.PeerOnly = 1
	} else if strings.TrimSpace(peer) == "" {
		d.LocalOnly = 1
	} else {
		d.Conflict = 1
	}
	return d
}

func pickXrayTemplate(source string, local, peer string) string {
	if source == AlignSourcePeer {
		return peer
	}
	return local
}

func diffGames(local, peer []GameBackup) PanelAlignCategoryDiff {
	lm, pm := map[string]GameBackup{}, map[string]GameBackup{}
	for _, g := range local {
		lm[strings.TrimSpace(g.Code)] = g
	}
	for _, g := range peer {
		pm[strings.TrimSpace(g.Code)] = g
	}
	return diffGameMaps(lm, pm)
}

func diffSocks(local, peer []SocksBackup) PanelAlignCategoryDiff {
	lm, pm := map[string]SocksBackup{}, map[string]SocksBackup{}
	for _, sp := range local {
		lm[SocksNaturalKey(sp.Address, sp.Port)] = sp
	}
	for _, sp := range peer {
		pm[SocksNaturalKey(sp.Address, sp.Port)] = sp
	}
	return diffSocksMaps(lm, pm)
}

func diffInbounds(local, peer []InboundBackup) PanelAlignCategoryDiff {
	lm, pm := map[int]InboundBackup{}, map[int]InboundBackup{}
	for _, ib := range local {
		lm[ib.Port] = ib
	}
	for _, ib := range peer {
		pm[ib.Port] = ib
	}
	d := PanelAlignCategoryDiff{}
	seen := map[int]bool{}
	for port, a := range lm {
		seen[port] = true
		b, ok := pm[port]
		if !ok {
			d.LocalOnly++
			continue
		}
		if !inboundSame(a, b) {
			d.Conflict++
		}
	}
	for port := range pm {
		if seen[port] {
			continue
		}
		d.PeerOnly++
	}
	return d
}

func inboundSame(a, b InboundBackup) bool {
	return a.Enable == b.Enable && a.ExpiryTime == b.ExpiryTime &&
		a.Listen == b.Listen && a.Protocol == b.Protocol && a.Settings == b.Settings &&
		a.StreamSettings == b.StreamSettings && a.Sniffing == b.Sniffing &&
		a.SocksKey == b.SocksKey && a.GameCode == b.GameCode &&
		a.RotationEnable == b.RotationEnable && a.RotationPolicy == b.RotationPolicy
}

func diffInboundRemarks(local, peer []InboundBackup) PanelAlignCategoryDiff {
	lm, pm := map[int]InboundBackup{}, map[int]InboundBackup{}
	for _, ib := range local {
		lm[ib.Port] = ib
	}
	for _, ib := range peer {
		pm[ib.Port] = ib
	}
	d := PanelAlignCategoryDiff{}
	for port, a := range lm {
		b, ok := pm[port]
		if !ok {
			continue
		}
		if strings.TrimSpace(a.Remark) != strings.TrimSpace(b.Remark) {
			d.Conflict++
		}
	}
	return d
}

func diffMarks(local, peer []SocksGameMarkBackup) PanelAlignCategoryDiff {
	lm, pm := map[string]SocksGameMarkBackup{}, map[string]SocksGameMarkBackup{}
	for _, mk := range local {
		lm[markKey(mk.SocksKey, mk.GameCode)] = mk
	}
	for _, mk := range peer {
		pm[markKey(mk.SocksKey, mk.GameCode)] = mk
	}
	return diffMarkMaps(lm, pm)
}

func markKey(socksKey, gameCode string) string {
	return socksKey + "|" + gameCode
}

func diffGameMaps(lm, pm map[string]GameBackup) PanelAlignCategoryDiff {
	d := PanelAlignCategoryDiff{}
	seen := map[string]bool{}
	for k, a := range lm {
		seen[k] = true
		b, ok := pm[k]
		if !ok {
			d.LocalOnly++
			continue
		}
		if a.Name != b.Name || a.Enable != b.Enable || a.SortOrder != b.SortOrder || a.Remark != b.Remark {
			d.Conflict++
		}
	}
	for k := range pm {
		if !seen[k] {
			d.PeerOnly++
		}
	}
	return d
}

func diffSocksMaps(lm, pm map[string]SocksBackup) PanelAlignCategoryDiff {
	d := PanelAlignCategoryDiff{}
	seen := map[string]bool{}
	for k, a := range lm {
		seen[k] = true
		b, ok := pm[k]
		if !ok {
			d.LocalOnly++
			continue
		}
		if a.Username != b.Username || a.Password != b.Password || a.Enable != b.Enable ||
			a.Remark != b.Remark || a.ExpiryTime != b.ExpiryTime {
			d.Conflict++
		}
	}
	for k := range pm {
		if !seen[k] {
			d.PeerOnly++
		}
	}
	return d
}

func diffMarkMaps(lm, pm map[string]SocksGameMarkBackup) PanelAlignCategoryDiff {
	d := PanelAlignCategoryDiff{}
	seen := map[string]bool{}
	for k, a := range lm {
		seen[k] = true
		b, ok := pm[k]
		if !ok {
			d.LocalOnly++
			continue
		}
		if a.Status != b.Status || a.BannedAt != b.BannedAt ||
			a.LastUsedAt != b.LastUsedAt || a.UseCount != b.UseCount {
			d.Conflict++
		}
	}
	for k := range pm {
		if !seen[k] {
			d.PeerOnly++
		}
	}
	return d
}

func pickCategoryGames(source string, local, peer []GameBackup) []GameBackup {
	switch source {
	case AlignSourcePeer:
		return peer
	default:
		return local
	}
}

func pickCategorySocks(source string, local, peer []SocksBackup) []SocksBackup {
	switch source {
	case AlignSourcePeer:
		return peer
	default:
		return local
	}
}

func pickCategoryInbounds(source string, local, peer []InboundBackup) []InboundBackup {
	switch source {
	case AlignSourcePeer:
		return copyInboundBackups(peer)
	default:
		return copyInboundBackups(local)
	}
}

func copyInboundBackups(list []InboundBackup) []InboundBackup {
	if len(list) == 0 {
		return nil
	}
	out := make([]InboundBackup, len(list))
	copy(out, list)
	return out
}

func pickInboundsWithRemarks(inboundsSource, remarkSource string, local, peer []InboundBackup) []InboundBackup {
	base := pickCategoryInbounds(inboundsSource, local, peer)
	remarkList := pickCategoryInbounds(remarkSource, local, peer)
	remarkByPort := make(map[int]string, len(remarkList))
	for _, ib := range remarkList {
		remarkByPort[ib.Port] = ib.Remark
	}
	for i := range base {
		if r, ok := remarkByPort[base[i].Port]; ok {
			base[i].Remark = r
		}
	}
	return base
}

func pickCategoryMarks(source string, local, peer []SocksGameMarkBackup) []SocksGameMarkBackup {
	switch source {
	case AlignSourcePeer:
		return peer
	case AlignSourceMerge:
		return mergeMarkSnapshots(local, peer)
	default:
		return local
	}
}

func mergeMarkSnapshots(local, peer []SocksGameMarkBackup) []SocksGameMarkBackup {
	m := make(map[string]SocksGameMarkBackup)
	for _, mk := range local {
		m[markKey(mk.SocksKey, mk.GameCode)] = mk
	}
	for _, pk := range peer {
		k := markKey(pk.SocksKey, pk.GameCode)
		if lk, ok := m[k]; ok {
			m[k] = mergeOneMark(lk, pk)
		} else {
			m[k] = pk
		}
	}
	out := make([]SocksGameMarkBackup, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func mergeOneMark(a, b SocksGameMarkBackup) SocksGameMarkBackup {
	out := a
	if a.Status == model.SocksGameStatusBanned || b.Status == model.SocksGameStatusBanned {
		out.Status = model.SocksGameStatusBanned
		if b.BannedAt > out.BannedAt {
			out.BannedAt = b.BannedAt
		}
		if a.BannedAt > out.BannedAt {
			out.BannedAt = a.BannedAt
		}
		return out
	}
	if a.Status == model.SocksGameStatusUsed || b.Status == model.SocksGameStatusUsed ||
		a.UseCount > 0 || b.UseCount > 0 {
		out.Status = model.SocksGameStatusUsed
		if a.UseCount > out.UseCount {
			out.UseCount = a.UseCount
		}
		if b.UseCount > out.UseCount {
			out.UseCount = b.UseCount
		}
		if a.LastUsedAt > out.LastUsedAt {
			out.LastUsedAt = a.LastUsedAt
		}
		if b.LastUsedAt > out.LastUsedAt {
			out.LastUsedAt = b.LastUsedAt
		}
		return out
	}
	if b.Status != "" {
		out.Status = b.Status
	}
	return out
}
