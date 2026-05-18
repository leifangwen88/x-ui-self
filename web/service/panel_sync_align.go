package service

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/util/common"

	"gorm.io/gorm"
)

func normalizePeerKey(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

func resolvePeerIndex(cfg *PanelSyncConfig, peerIndex int, peerBaseURL string) (int, error) {
	if cfg == nil {
		return -1, common.NewError("配置为空")
	}
	if peerIndex >= 0 && peerIndex < len(cfg.Peers) {
		return peerIndex, nil
	}
	key := normalizePeerKey(peerBaseURL)
	if key == "" {
		return -1, common.NewError("无效的对等节点索引")
	}
	for i, p := range cfg.Peers {
		if normalizePeerKey(p.BaseURL) == key {
			return i, nil
		}
	}
	return -1, common.NewError("无效的对等节点索引")
}

func (s *PanelSyncService) ListPeerAlignStatus(cfg *PanelSyncConfig) []PanelPeerAlignStatus {
	if cfg == nil {
		return nil
	}
	db := database.GetDB()
	list := make([]PanelPeerAlignStatus, 0, len(cfg.Peers))
	for _, p := range cfg.Peers {
		key := normalizePeerKey(p.BaseURL)
		if key == "" {
			continue
		}
		cur := &model.SyncPeerCursor{PeerKey: key}
		_ = db.Where("peer_key = ?", key).First(cur).Error
		list = append(list, PanelPeerAlignStatus{
			PeerKey:   key,
			Name:      p.Name,
			BaseURL:   p.BaseURL,
			AlignedAt: cur.AlignedAt,
		})
	}
	return list
}

func (s *PanelSyncService) CompareWithPeer(peerIndex int, peerBaseURL string) (*PanelAlignCompareResult, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	idx, err := resolvePeerIndex(cfg, peerIndex, peerBaseURL)
	if err != nil {
		return nil, err
	}
	peer := cfg.Peers[idx]
	peerKey := normalizePeerKey(peer.BaseURL)
	if peerKey == "" {
		return nil, common.NewError("对等节点地址为空")
	}

	local, err := s.BuildLocalSnapshot()
	if err != nil {
		return nil, err
	}

	res := &PanelAlignCompareResult{
		PeerKey:  peerKey,
		PeerName: peer.Name,
		Local:    snapshotCounts(local),
	}

	db := database.GetDB()
	cur := &model.SyncPeerCursor{PeerKey: peerKey}
	_ = db.Where("peer_key = ?", peerKey).First(cur).Error
	res.AlignedAt = cur.AlignedAt

	secret := strings.TrimSpace(peer.Secret)
	if secret == "" {
		secret = cfg.Secret
	}
	remote, err := fetchPeerSnapshot(peer, secret)
	if err != nil {
		res.PeerReachable = false
		res.PeerError = err.Error()
		res.Diff = compareSnapshots(local, &PanelSyncSnapshot{})
		return res, nil
	}
	res.PeerReachable = true
	res.Peer = snapshotCounts(remote)
	res.Diff = compareSnapshots(local, remote)
	if alignDiffEmpty(res.Diff) {
		alignedAt := time.Now().UnixMilli()
		s.markPeerAligned(peerKey, alignedAt)
		res.AlignedAt = alignedAt
	}
	return res, nil
}

func (s *PanelSyncService) ApplyClusterAlign(userId int, req *PanelAlignApplyRequest) (*PanelAlignApplyResult, error) {
	if req == nil {
		return nil, common.NewError("请求为空")
	}
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	idx, err := resolvePeerIndex(cfg, req.PeerIndex, req.PeerBaseURL)
	if err != nil {
		return nil, err
	}
	peer := cfg.Peers[idx]
	peerKey := normalizePeerKey(peer.BaseURL)
	if peerKey == "" {
		return nil, common.NewError("对等节点地址为空")
	}

	if err := validateAlignSource(req.GamesSource); err != nil {
		return nil, err
	}
	if err := validateAlignSource(req.SocksSource); err != nil {
		return nil, err
	}
	if err := validateAlignSource(req.InboundsSource); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.InboundRemarkSource) == "" {
		req.InboundRemarkSource = req.InboundsSource
	}
	if err := validateAlignSource(req.InboundRemarkSource); err != nil {
		return nil, err
	}
	if err := validateMarkAlignSource(req.MarksSource); err != nil {
		return nil, err
	}
	if err := validateAlignSource(req.XrayTemplateSource); err != nil {
		return nil, err
	}

	localSnap, err := s.BuildLocalSnapshot()
	if err != nil {
		return nil, err
	}
	secret := strings.TrimSpace(peer.Secret)
	if secret == "" {
		secret = cfg.Secret
	}
	peerSnap, err := fetchPeerSnapshot(peer, secret)
	if err != nil {
		return nil, common.NewError("无法拉取对端快照:", err)
	}

	final := &PanelSyncSnapshot{
		NodeId:             cfg.NodeId,
		ExportedAt:         time.Now().UnixMilli(),
		Games:              pickCategoryGames(req.GamesSource, localSnap.Games, peerSnap.Games),
		SocksProxies:       pickCategorySocks(req.SocksSource, localSnap.SocksProxies, peerSnap.SocksProxies),
		Inbounds:           pickInboundsWithRemarks(req.InboundsSource, req.InboundRemarkSource, localSnap.Inbounds, peerSnap.Inbounds),
		SocksGameMarks:     pickCategoryMarks(req.MarksSource, localSnap.SocksGameMarks, peerSnap.SocksGameMarks),
		XrayTemplateConfig: pickXrayTemplate(req.XrayTemplateSource, localSnap.XrayTemplateConfig, peerSnap.XrayTemplateConfig),
	}

	atomic.AddInt32(&s.applying, 1)
	defer atomic.AddInt32(&s.applying, -1)

	if err := applySyncSnapshot(userId, final); err != nil {
		return nil, err
	}

	alignedAt := time.Now().UnixMilli()
	s.markPeerAligned(peerKey, alignedAt)

	result := &PanelAlignApplyResult{AlignedAt: alignedAt}
	if req.PushToPeer {
		if err := postPeerAlignApply(peer, secret, final, alignedAt, cfg.PublicURL, AlignScopeFull); err != nil {
			result.PeerPushErr = err.Error()
		} else {
			result.PeerPushed = true
		}
	}
	s.xrayService.SetToNeedRestart()
	return result, nil
}

func (s *PanelSyncService) ReceiveAlignApply(secret string, snap *PanelSyncSnapshot, alignedAt int64, originBaseURL string, scope string) error {
	cfg, err := s.GetConfig()
	if err != nil {
		return err
	}
	if strings.TrimSpace(secret) == "" || secret != cfg.Secret {
		return common.NewError("同步密钥无效")
	}
	if snap == nil {
		return common.NewError("对齐快照为空")
	}

	userId, err := primaryUserID()
	if err != nil {
		return err
	}

	atomic.AddInt32(&s.applying, 1)
	defer atomic.AddInt32(&s.applying, -1)

	_ = scope
	if err := applySyncSnapshot(userId, snap); err != nil {
		return err
	}
	if alignedAt <= 0 {
		alignedAt = time.Now().UnixMilli()
	}
	originBaseURL = normalizePeerBaseURL(originBaseURL)
	if originBaseURL != "" {
		s.markPeerAligned(originBaseURL, alignedAt)
		members, _ := s.loadClusterMembers()
		members = mergeClusterMembers(members, []ClusterMember{{
			PublicURL: originBaseURL,
			Name:      "对端节点",
			UpdatedAt: alignedAt,
		}})
		_ = s.persistMembersAndPeers(cfg, members, false)
	}
	s.xrayService.SetToNeedRestart()
	return nil
}

func (s *PanelSyncService) markPeerAligned(peerKey string, alignedAt int64) {
	db := database.GetDB()
	cur := model.SyncPeerCursor{PeerKey: peerKey}
	_ = db.Where("peer_key = ?", peerKey).First(&cur).Error
	cur.AlignedAt = alignedAt
	if cur.LastPull < alignedAt {
		cur.LastPull = alignedAt
	}
	_ = db.Save(&cur).Error
}

func alignDiffEmpty(diff PanelAlignDiff) bool {
	categories := []PanelAlignCategoryDiff{
		diff.Games,
		diff.Socks,
		diff.Inbounds,
		diff.InboundRemarks,
		diff.Marks,
		diff.XrayTemplate,
	}
	for _, d := range categories {
		if d.LocalOnly != 0 || d.PeerOnly != 0 || d.Conflict != 0 {
			return false
		}
	}
	return true
}

func validateAlignSource(src string) error {
	switch src {
	case AlignSourceLocal, AlignSourcePeer:
		return nil
	default:
		return common.NewError("无效对齐来源:", src, "（游戏/SOCKS/入站请选 local 或 peer）")
	}
}

func validateMarkAlignSource(src string) error {
	switch src {
	case AlignSourceLocal, AlignSourcePeer, AlignSourceMerge:
		return nil
	default:
		return common.NewError("无效标记对齐来源:", src)
	}
}

func primaryUserID() (int, error) {
	db := database.GetDB()
	u := &model.User{}
	if err := db.Order("id asc").First(u).Error; err != nil {
		return 0, common.NewError("无法获取管理员用户")
	}
	return u.Id, nil
}

func applySyncSnapshot(userId int, snap *PanelSyncSnapshot) error {
	if snap == nil {
		return common.NewError("快照为空")
	}
	db := database.GetDB()
	settingSvc := SettingService{}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&model.SocksRotationLog{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&model.SocksGameStatus{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&model.Inbound{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&model.SocksProxy{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&model.Game{}).Error; err != nil {
			return err
		}

		gameIDByCode := make(map[string]int)
		for _, g := range snap.Games {
			name := strings.TrimSpace(g.Name)
			if name == "" {
				return common.NewError("快照中存在空游戏名称")
			}
			code := strings.TrimSpace(g.Code)
			if code == "" {
				code = fmt.Sprintf("game_%d", time.Now().UnixNano())
			}
			row := &model.Game{
				Name:      name,
				Code:      code,
				Enable:    g.Enable,
				SortOrder: g.SortOrder,
				Remark:    g.Remark,
			}
			if err := tx.Create(row).Error; err != nil {
				return err
			}
			gameIDByCode[code] = row.Id
		}

		socksIDByKey := make(map[string]int)
		for _, sp := range snap.SocksProxies {
			key := SocksNaturalKey(sp.Address, sp.Port)
			if key == ":0" {
				continue
			}
			row := &model.SocksProxy{
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
			if err := tx.Create(row).Error; err != nil {
				return common.NewError("写入 SOCKS 失败 ", key, ": ", err)
			}
			socksIDByKey[key] = row.Id
		}

		for _, ib := range snap.Inbounds {
			if ib.Port <= 0 || ib.Port > 65535 {
				return common.NewError("无效入站端口:", ib.Port)
			}
			policy := ib.RotationPolicy
			if policy == "" {
				policy = model.RotationPolicyPreferUnusedUnbanned
			}
			row := &model.Inbound{
				UserId:         userId,
				Up:             ib.Up,
				Down:           ib.Down,
				Total:          ib.Total,
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
				RotationEnable: ib.RotationEnable,
				RotationPolicy: policy,
				LastRotatedAt:  ib.LastRotatedAt,
			}
			if ib.SocksKey != "" {
				sid, ok := socksIDByKey[ib.SocksKey]
				if !ok {
					return common.NewError("入站端口 ", ib.Port, " 引用了不存在的 SOCKS: ", ib.SocksKey)
				}
				row.SocksProxyId = sid
			}
			if ib.GameCode != "" {
				gid, ok := gameIDByCode[ib.GameCode]
				if !ok {
					return common.NewError("入站端口 ", ib.Port, " 引用了不存在的游戏: ", ib.GameCode)
				}
				row.GameId = gid
			}
			if err := tx.Create(row).Error; err != nil {
				return common.NewError("写入入站 ", ib.Port, " 失败: ", err)
			}
		}

		for _, mk := range snap.SocksGameMarks {
			gid, ok := gameIDByCode[mk.GameCode]
			if !ok {
				continue
			}
			addr, port, okKey := ParseSocksNaturalKey(mk.SocksKey)
			sid := 0
			if id, ok := socksIDByKey[mk.SocksKey]; ok {
				sid = id
			}
			status := mk.Status
			if status == "" {
				status = model.SocksGameStatusActive
			}
			row := &model.SocksGameStatus{
				SocksProxyId: sid,
				GameId:       gid,
				Status:       status,
				BannedAt:     mk.BannedAt,
				LastUsedAt:   mk.LastUsedAt,
				UseCount:     mk.UseCount,
				Note:         mk.Note,
			}
			if okKey {
				row.SocksAddress = addr
				row.SocksPort = port
			}
			if err := tx.Create(row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return settingSvc.setString("xrayTemplateConfig", snap.XrayTemplateConfig)
}

