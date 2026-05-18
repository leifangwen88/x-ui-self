package service

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/util/common"
	"x-ui/util/random"

	"gorm.io/gorm"
)

const (
	SyncEventMarkUsed         = "game_mark_used"
	SyncEventMarkBanned       = "game_mark_banned"
	SyncEventMarkClear        = "game_mark_clear"
	SyncEventInboundBindSocks = "inbound_bind_socks"
	SyncEventInboundBindGame  = "inbound_bind_game"
	SyncEventInboundRemark    = "inbound_remark"
	SyncEventSocksDelete      = "socks_delete"
)

type SyncPeerConfig struct {
	Name     string `json:"name" form:"name"`
	BaseURL  string `json:"baseUrl" form:"baseUrl"`
	Secret   string `json:"secret" form:"secret"`
	Fallback bool   `json:"fallback" form:"fallback"` // 兜底节点：高价稳定，仅当首选不可用时使用
}

type PanelSyncConfig struct {
	Enabled       bool                   `json:"enabled" form:"enabled"`
	NodeId        string                 `json:"nodeId" form:"nodeId"`
	Secret        string                 `json:"secret" form:"secret"`
	PublicURL     string                 `json:"publicUrl" form:"publicUrl"`
	LocalFallback bool                   `json:"localFallback" form:"localFallback"` // 本机在站群订阅中作为兜底节点
	Peers         []SyncPeerConfig       `json:"peers" form:"peers"`
	Members       []ClusterMember        `json:"members,omitempty"`
	PeerStatus    []PanelPeerAlignStatus `json:"peerStatus,omitempty" form:"-"`
}

type SyncEventDTO struct {
	EventId   string          `json:"eventId"`
	NodeId    string          `json:"nodeId"`
	CreatedAt int64           `json:"createdAt"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type syncMarkPayload struct {
	SocksKey string `json:"socksKey"`
	GameCode string `json:"gameCode"`
	Note     string `json:"note"`
}

type syncInboundSocksPayload struct {
	InboundPort int    `json:"inboundPort"`
	SocksKey    string `json:"socksKey"`
}

type syncInboundGamePayload struct {
	InboundPort int    `json:"inboundPort"`
	GameCode    string `json:"gameCode"`
}

type syncInboundRemarkPayload struct {
	InboundPort int    `json:"inboundPort"`
	Remark      string `json:"remark"`
}

type syncSocksDeletePayload struct {
	SocksKey string `json:"socksKey"`
}

type PanelSyncService struct {
	settingService SettingService
	xrayService    XrayService
	applying       int32
}

var globalPanelSync *PanelSyncService

func NewPanelSyncService(xray XrayService, setting SettingService) *PanelSyncService {
	return &PanelSyncService{
		xrayService:    xray,
		settingService: setting,
	}
}

func InitPanelSync(s *PanelSyncService) {
	globalPanelSync = s
}

func GetPanelSync() *PanelSyncService {
	return globalPanelSync
}

func (s *PanelSyncService) IsApplying() bool {
	return atomic.LoadInt32(&s.applying) > 0
}

func syncEmit(typ string, payload interface{}) {
	if globalPanelSync == nil || globalPanelSync.IsApplying() {
		return
	}
	_ = globalPanelSync.Emit(typ, payload)
}

func (s *PanelSyncService) GetConfig() (*PanelSyncConfig, error) {
	cfg := &PanelSyncConfig{Peers: []SyncPeerConfig{}}
	nodeId, err := s.settingService.getString("panelNodeId")
	if err != nil && !database.IsNotFound(err) {
		return nil, err
	}
	if strings.TrimSpace(nodeId) == "" {
		nodeId = "node_" + random.Seq(16)
		_ = s.settingService.setString("panelNodeId", nodeId)
	}
	cfg.NodeId = nodeId
	secret, _ := s.settingService.getString("panelSyncSecret")
	if strings.TrimSpace(secret) == "" {
		secret = random.Seq(32)
		_ = s.settingService.setString("panelSyncSecret", secret)
	}
	cfg.Secret = secret
	enabled, _ := s.settingService.getBool("panelSyncEnabled")
	cfg.Enabled = enabled
	publicURL, _ := s.settingService.getString("panelSyncPublicURL")
	cfg.PublicURL = strings.TrimSpace(publicURL)
	localFallback, _ := s.settingService.getBool("panelSyncLocalFallback")
	cfg.LocalFallback = localFallback
	raw, _ := s.settingService.getString("panelSyncPeers")
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &cfg.Peers)
	}
	if cfg.Peers == nil {
		cfg.Peers = []SyncPeerConfig{}
	}
	rawMembers, _ := s.loadClusterMembers()
	members := s.migratePeersToMembersIfNeeded(cfg, rawMembers)
	sanitized := s.sanitizeClusterMembers(cfg, members)
	if len(sanitized) != len(members) || hasMislabeledLocalMembers(members, cfg) {
		_ = s.saveClusterMembers(sanitized)
		_ = s.rebuildPeersFromMembers(cfg, sanitized)
	}
	members = sanitized
	cfg.Members = members
	cfg.Peers = s.membersToPeers(cfg, members)
	cfg.PeerStatus = s.ListPeerAlignStatus(cfg)
	return cfg, nil
}

func (s *PanelSyncService) SaveConfig(cfg *PanelSyncConfig) error {
	if cfg == nil {
		return common.NewError("配置为空")
	}
	if strings.TrimSpace(cfg.NodeId) == "" {
		cfg.NodeId = "node_" + random.Seq(16)
	}
	if strings.TrimSpace(cfg.Secret) == "" {
		cfg.Secret = random.Seq(32)
	}
	_ = s.settingService.setString("panelNodeId", strings.TrimSpace(cfg.NodeId))
	_ = s.settingService.setString("panelSyncSecret", strings.TrimSpace(cfg.Secret))
	_ = s.settingService.setString("panelSyncPublicURL", strings.TrimSpace(cfg.PublicURL))
	_ = s.settingService.setBool("panelSyncEnabled", cfg.Enabled)
	_ = s.settingService.setBool("panelSyncLocalFallback", cfg.LocalFallback)
	members, _ := s.loadClusterMembers()
	members = s.migratePeersToMembersIfNeeded(cfg, members)
	if len(cfg.Members) > 0 {
		members = mergeClusterMembers(members, cfg.Members)
	} else if len(cfg.Peers) > 0 {
		now := time.Now().UnixMilli()
		for _, p := range cfg.Peers {
			url := normalizePeerKey(p.BaseURL)
			if url == "" {
				continue
			}
			members = mergeClusterMembers(members, []ClusterMember{{
				PublicURL: url,
				Name:      p.Name,
				Fallback:  p.Fallback,
				UpdatedAt: now,
			}})
		}
	}
	return s.persistMembersAndPeers(cfg, members, cfg.Enabled)
}

func (s *PanelSyncService) Emit(typ string, payload interface{}) error {
	cfg, err := s.GetConfig()
	if err != nil || !cfg.Enabled || len(cfg.Peers) == 0 {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	evt := &model.SyncOutbox{
		EventId:   "evt_" + random.Seq(24),
		NodeId:    cfg.NodeId,
		CreatedAt: time.Now().UnixMilli(),
		Type:      typ,
		Payload:   string(raw),
	}
	db := database.GetDB()
	if err := db.Create(evt).Error; err != nil {
		return err
	}
	go s.pushEventToPeers(cfg, SyncEventDTO{
		EventId:   evt.EventId,
		NodeId:    evt.NodeId,
		CreatedAt: evt.CreatedAt,
		Type:      evt.Type,
		Payload:   raw,
	})
	return nil
}

func (s *PanelSyncService) ListOutboxSince(since int64, limit int) ([]SyncEventDTO, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	db := database.GetDB()
	var rows []*model.SyncOutbox
	q := db.Order("created_at asc").Limit(limit)
	if since > 0 {
		q = q.Where("created_at > ?", since)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	list := make([]SyncEventDTO, 0, len(rows))
	for _, r := range rows {
		list = append(list, SyncEventDTO{
			EventId:   r.EventId,
			NodeId:    r.NodeId,
			CreatedAt: r.CreatedAt,
			Type:      r.Type,
			Payload:   json.RawMessage(r.Payload),
		})
	}
	return list, nil
}

func (s *PanelSyncService) ReceiveEvent(secret string, evt SyncEventDTO) error {
	cfg, err := s.GetConfig()
	if err != nil {
		return err
	}
	if strings.TrimSpace(secret) == "" || secret != cfg.Secret {
		return common.NewError("同步密钥无效")
	}
	if evt.EventId == "" || evt.NodeId == "" || evt.Type == "" {
		return common.NewError("事件字段不完整")
	}
	if evt.NodeId == cfg.NodeId {
		return nil
	}
	db := database.GetDB()
	var exist model.SyncReceived
	if err := db.Where("event_id = ?", evt.EventId).First(&exist).Error; err == nil {
		return nil
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	return s.ApplyEvent(evt)
}

func (s *PanelSyncService) ApplyEvent(evt SyncEventDTO) error {
	atomic.AddInt32(&s.applying, 1)
	defer atomic.AddInt32(&s.applying, -1)

	var err error
	switch evt.Type {
	case SyncEventMarkUsed:
		err = s.applyMarkUsed(evt.Payload)
	case SyncEventMarkBanned:
		err = s.applyMarkBanned(evt.Payload)
	case SyncEventMarkClear:
		err = s.applyMarkClear(evt.Payload)
	case SyncEventInboundBindSocks:
		err = s.applyInboundBindSocks(evt.Payload)
	case SyncEventInboundBindGame:
		err = s.applyInboundBindGame(evt.Payload)
	case SyncEventInboundRemark:
		err = s.applyInboundRemark(evt.Payload)
	case SyncEventSocksDelete:
		err = s.applySocksDelete(evt.Payload)
	case SyncEventGameUpsert:
		err = s.applyGameUpsert(evt.Payload)
	case SyncEventGameDelete:
		err = s.applyGameDelete(evt.Payload)
	case SyncEventSocksUpsert:
		err = s.applySocksUpsert(evt.Payload)
	case SyncEventInboundUpsert:
		err = s.applyInboundUpsert(evt.Payload)
	case SyncEventInboundDelete:
		err = s.applyInboundDelete(evt.Payload)
	case SyncEventMarkUnban:
		err = s.applyMarkUnban(evt.Payload)
	case SyncEventXrayTemplate:
		err = s.applyXrayTemplate(evt.Payload)
	case SyncEventClusterMemberUpsert:
		err = s.applyClusterMemberUpsert(evt.Payload)
	case SyncEventClusterMemberRemove:
		err = s.applyClusterMemberRemove(evt.Payload)
	default:
		return nil
	}
	if err != nil {
		return err
	}
	db := database.GetDB()
	_ = db.Create(&model.SyncReceived{
		EventId:    evt.EventId,
		FromNodeId: evt.NodeId,
		ReceivedAt: time.Now().UnixMilli(),
	}).Error
	return nil
}

func (s *PanelSyncService) applyMarkUsed(raw json.RawMessage) error {
	var p syncMarkPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	gameID, err := s.gameIDByCode(p.GameCode)
	if err != nil {
		return err
	}
	socksID, addr, port, _ := s.resolveSocks(p.SocksKey)
	if socksID > 0 {
		sg := SocksGameService{}
		if sg.IsBanned(socksID, gameID) {
			return nil
		}
		return sg.MarkUsed(socksID, gameID, p.Note)
	}
	return s.upsertOrphanMark(addr, port, gameID, model.SocksGameStatusUsed, p.Note, false)
}

func (s *PanelSyncService) applyMarkBanned(raw json.RawMessage) error {
	var p syncMarkPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	gameID, err := s.gameIDByCode(p.GameCode)
	if err != nil {
		return err
	}
	socksID, addr, port, _ := s.resolveSocks(p.SocksKey)
	if socksID > 0 {
		sg := SocksGameService{}
		return sg.MarkBanned(socksID, gameID, p.Note)
	}
	return s.upsertOrphanMark(addr, port, gameID, model.SocksGameStatusBanned, p.Note, true)
}

func (s *PanelSyncService) applyMarkClear(raw json.RawMessage) error {
	var p syncMarkPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	gameID, err := s.gameIDByCode(p.GameCode)
	if err != nil {
		return err
	}
	addr, port, ok := ParseSocksNaturalKey(p.SocksKey)
	if !ok {
		return nil
	}
	db := database.GetDB()
	return db.Where("game_id = ? AND socks_address = ? AND socks_port = ?", gameID, addr, port).
		Delete(&model.SocksGameStatus{}).Error
}

func (s *PanelSyncService) applyInboundBindSocks(raw json.RawMessage) error {
	var p syncInboundSocksPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	inbound, err := s.inboundByPort(p.InboundPort)
	if err != nil {
		return nil
	}
	socksID := 0
	if strings.TrimSpace(p.SocksKey) != "" {
		id, _, _, ok := s.resolveSocks(p.SocksKey)
		if !ok {
			return nil
		}
		socksID = id
	}
	inboundSvc := InboundService{}
	if err := inboundSvc.UpdateSocksProxyId(inbound.Id, socksID); err != nil {
		return err
	}
	s.xrayService.SetToNeedRestart()
	return nil
}

func (s *PanelSyncService) applyInboundRemark(raw json.RawMessage) error {
	var p syncInboundRemarkPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	if p.InboundPort <= 0 {
		return nil
	}
	inbound, err := s.inboundByPort(p.InboundPort)
	if err != nil {
		return nil
	}
	db := database.GetDB()
	return db.Model(model.Inbound{}).Where("id = ?", inbound.Id).Update("remark", p.Remark).Error
}

func (s *PanelSyncService) applyInboundBindGame(raw json.RawMessage) error {
	var p syncInboundGamePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	inbound, err := s.inboundByPort(p.InboundPort)
	if err != nil {
		return nil
	}
	gameID := 0
	if strings.TrimSpace(p.GameCode) != "" {
		id, err := s.gameIDByCode(p.GameCode)
		if err != nil {
			return nil
		}
		gameID = id
	}
	inboundSvc := InboundService{}
	return inboundSvc.UpdateGameId(inbound.Id, gameID)
}

func (s *PanelSyncService) applySocksDelete(raw json.RawMessage) error {
	var p syncSocksDeletePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	if err := DeleteSocksByNaturalKey(strings.TrimSpace(p.SocksKey), false); err != nil {
		return err
	}
	s.xrayService.SetToNeedRestart()
	return nil
}

func (s *PanelSyncService) gameIDByCode(code string) (int, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return 0, common.NewError("gameCode 为空")
	}
	db := database.GetDB()
	g := &model.Game{}
	if err := db.Where("code = ?", code).First(g).Error; err != nil {
		return 0, err
	}
	return g.Id, nil
}

func (s *PanelSyncService) inboundByPort(port int) (*model.Inbound, error) {
	db := database.GetDB()
	ib := &model.Inbound{}
	err := db.Where("port = ?", port).First(ib).Error
	return ib, err
}

func (s *PanelSyncService) resolveSocks(key string) (id int, address string, port int, ok bool) {
	address, port, ok = ParseSocksNaturalKey(key)
	if !ok {
		return 0, "", 0, false
	}
	address = strings.TrimSpace(address)
	db := database.GetDB()
	sp := &model.SocksProxy{}
	err := db.Where("address = ? AND port = ?", address, port).First(sp).Error
	if err != nil {
		return 0, address, port, false
	}
	return sp.Id, address, port, true
}

func (s *PanelSyncService) upsertOrphanMark(address string, port int, gameID int, status string, note string, banned bool) error {
	db := database.GetDB()
	now := time.Now().UnixMilli()
	st := &model.SocksGameStatus{}
	err := db.Where("game_id = ? AND socks_address = ? AND socks_port = ?", gameID, address, port).First(st).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	if err == gorm.ErrRecordNotFound {
		st = &model.SocksGameStatus{
			SocksProxyId: 0,
			GameId:       gameID,
			SocksAddress: address,
			SocksPort:    port,
			Status:       status,
			Note:         note,
		}
		if banned {
			st.BannedAt = now
		} else {
			st.LastUsedAt = now
			st.UseCount = 1
		}
		return db.Create(st).Error
	}
	if banned {
		st.Status = model.SocksGameStatusBanned
		if st.BannedAt <= 0 {
			st.BannedAt = now
		}
	} else if st.Status != model.SocksGameStatusBanned {
		st.Status = model.SocksGameStatusUsed
		st.LastUsedAt = now
		if st.UseCount < 1 {
			st.UseCount = 1
		}
	}
	if note != "" {
		st.Note = note
	}
	return db.Save(st).Error
}

func (s *PanelSyncService) gameCodeByID(gameID int) (string, error) {
	if gameID <= 0 {
		return "", nil
	}
	db := database.GetDB()
	g := &model.Game{}
	if err := db.First(g, gameID).Error; err != nil {
		return "", err
	}
	return strings.TrimSpace(g.Code), nil
}

func (s *PanelSyncService) EmitGameMark(socksProxyId, gameId int, mark string, note string) {
	if socksProxyId <= 0 || gameId <= 0 {
		return
	}
	db := database.GetDB()
	sp := &model.SocksProxy{}
	if err := db.First(sp, socksProxyId).Error; err != nil {
		return
	}
	code, err := s.gameCodeByID(gameId)
	if err != nil || code == "" {
		return
	}
	payload := syncMarkPayload{
		SocksKey: SocksNaturalKey(sp.Address, sp.Port),
		GameCode: code,
		Note:     note,
	}
	switch mark {
	case model.SocksGameMarkUsed:
		_ = s.Emit(SyncEventMarkUsed, payload)
	case model.SocksGameMarkBanned:
		_ = s.Emit(SyncEventMarkBanned, payload)
	case "clear":
		_ = s.Emit(SyncEventMarkClear, payload)
	}
}

func (s *PanelSyncService) EmitInboundBindSocks(inboundId int, socksProxyId int) {
	inboundSvc := InboundService{}
	ib, err := inboundSvc.GetInbound(inboundId)
	if err != nil {
		return
	}
	key := ""
	if socksProxyId > 0 {
		db := database.GetDB()
		sp := &model.SocksProxy{}
		if err := db.First(sp, socksProxyId).Error; err == nil {
			key = SocksNaturalKey(sp.Address, sp.Port)
		}
	}
	syncEmit(SyncEventInboundBindSocks, syncInboundSocksPayload{
		InboundPort: ib.Port,
		SocksKey:    key,
	})
}

func (s *PanelSyncService) EmitInboundRemark(inboundId int, remark string) {
	inboundSvc := InboundService{}
	ib, err := inboundSvc.GetInbound(inboundId)
	if err != nil {
		return
	}
	syncEmit(SyncEventInboundRemark, syncInboundRemarkPayload{
		InboundPort: ib.Port,
		Remark:      remark,
	})
}

func (s *PanelSyncService) EmitInboundBindGame(inboundId int, gameId int) {
	inboundSvc := InboundService{}
	ib, err := inboundSvc.GetInbound(inboundId)
	if err != nil {
		return
	}
	code := ""
	if gameId > 0 {
		code, _ = s.gameCodeByID(gameId)
	}
	syncEmit(SyncEventInboundBindGame, syncInboundGamePayload{
		InboundPort: ib.Port,
		GameCode:    code,
	})
}

func (s *PanelSyncService) EmitSocksDelete(address string, port int) {
	if strings.TrimSpace(address) == "" || port <= 0 {
		return
	}
	// 显式删除操作：直接写入 outbox 并推送，不受 IsApplying 影响
	_ = s.Emit(SyncEventSocksDelete, syncSocksDeletePayload{
		SocksKey: SocksNaturalKey(address, port),
	})
}

func (s *PanelSyncService) RunSyncCycle() {
	cfg, err := s.GetConfig()
	if err != nil || !cfg.Enabled {
		return
	}
	s.PushToPeers(cfg)
	s.PullFromPeers(cfg)
}

func (s *PanelSyncService) PushToPeers(cfg *PanelSyncConfig) {
	if cfg == nil {
		var err error
		cfg, err = s.GetConfig()
		if err != nil || !cfg.Enabled {
			return
		}
	}
	events, err := s.ListOutboxSince(0, 200)
	if err != nil {
		return
	}
	for _, evt := range events {
		s.pushEventToPeers(cfg, evt)
	}
}

func (s *PanelSyncService) PullFromPeers(cfg *PanelSyncConfig) {
	if cfg == nil {
		var err error
		cfg, err = s.GetConfig()
		if err != nil || !cfg.Enabled {
			return
		}
	}
	for _, peer := range cfg.Peers {
		s.pullPeer(cfg, peer)
	}
}

func (s *PanelSyncService) pullPeer(local *PanelSyncConfig, peer SyncPeerConfig) {
	peerKey := normalizePeerKey(peer.BaseURL)
	if peerKey == "" {
		return
	}
	db := database.GetDB()
	cursor := &model.SyncPeerCursor{PeerKey: peerKey}
	_ = db.Where("peer_key = ?", peerKey).First(cursor).Error
	if cursor.AlignedAt <= 0 {
		return
	}
	since := cursor.LastPull
	events, err := fetchPeerOutbox(peer, since, local.Secret)
	if err != nil {
		return
	}
	maxTs := since
	sortSyncEventsByPriority(events)
	for _, evt := range events {
		if evt.NodeId == local.NodeId {
			continue
		}
		_ = s.ApplyEvent(evt)
		if evt.CreatedAt > maxTs {
			maxTs = evt.CreatedAt
		}
	}
	if maxTs > since {
		cursor.LastPull = maxTs
		_ = db.Save(cursor).Error
	}
}

func (s *PanelSyncService) pushEventToPeers(cfg *PanelSyncConfig, evt SyncEventDTO) {
	for _, peer := range cfg.Peers {
		secret := strings.TrimSpace(peer.Secret)
		if secret == "" {
			secret = cfg.Secret
		}
		_ = postPeerEvent(peer, secret, evt)
	}
}