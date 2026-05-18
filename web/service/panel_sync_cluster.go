package service

import (
	"encoding/json"
	"strings"
	"time"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/util/common"
)

const (
	SyncEventClusterMemberUpsert = "cluster_member_upsert"
	SyncEventClusterMemberRemove = "cluster_member_remove"
)

// ClusterMember 站群成员公告（各节点维护一份，通过对等事件与种子拉取保持一致）
type ClusterMember struct {
	NodeId    string `json:"nodeId"`
	PublicURL string `json:"publicUrl"`
	Name      string `json:"name"`
	Fallback  bool   `json:"fallback"`
	UpdatedAt int64  `json:"updatedAt"`
}

type clusterMemberPayload struct {
	NodeId    string `json:"nodeId"`
	PublicURL string `json:"publicUrl"`
	Name      string `json:"name"`
	Fallback  bool   `json:"fallback"`
	UpdatedAt int64  `json:"updatedAt"`
}

type clusterMemberRemovePayload struct {
	NodeId    string `json:"nodeId"`
	PublicURL string `json:"publicUrl"`
}

type ClusterMembersResponse struct {
	SelfNodeId string          `json:"selfNodeId"`
	Members    []ClusterMember `json:"members"`
}

func (s *PanelSyncService) loadClusterMembers() ([]ClusterMember, error) {
	raw, _ := s.settingService.getString("panelSyncMembers")
	list := make([]ClusterMember, 0)
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &list)
	}
	if list == nil {
		list = []ClusterMember{}
	}
	return list, nil
}

func (s *PanelSyncService) saveClusterMembers(list []ClusterMember) error {
	if list == nil {
		list = []ClusterMember{}
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return s.settingService.setString("panelSyncMembers", string(raw))
}

func memberKey(m ClusterMember) string {
	if id := strings.TrimSpace(m.NodeId); id != "" {
		return "id:" + id
	}
	return "url:" + normalizePeerKey(m.PublicURL)
}

func mergeClusterMembers(base []ClusterMember, incoming []ClusterMember) []ClusterMember {
	index := make(map[string]ClusterMember)
	order := make([]string, 0)
	add := func(m ClusterMember) {
		url := normalizePeerKey(m.PublicURL)
		if url == "" && strings.TrimSpace(m.NodeId) == "" {
			return
		}
		m.PublicURL = url
		if m.UpdatedAt <= 0 {
			m.UpdatedAt = time.Now().UnixMilli()
		}
		k := memberKey(m)
		if prev, ok := index[k]; ok {
			if m.UpdatedAt >= prev.UpdatedAt {
				index[k] = m
			}
			return
		}
		index[k] = m
		order = append(order, k)
	}
	for _, m := range base {
		add(m)
	}
	for _, m := range incoming {
		add(m)
	}
	out := make([]ClusterMember, 0, len(order))
	for _, k := range order {
		out = append(out, index[k])
	}
	return out
}

func (s *PanelSyncService) membersToPeers(cfg *PanelSyncConfig, members []ClusterMember) []SyncPeerConfig {
	selfURL := normalizePeerKey(cfg.PublicURL)
	peers := make([]SyncPeerConfig, 0, len(members))
	for _, m := range members {
		url := normalizePeerKey(m.PublicURL)
		if url == "" || url == selfURL {
			continue
		}
		if cfg.NodeId != "" && strings.TrimSpace(m.NodeId) == cfg.NodeId {
			continue
		}
		peers = append(peers, SyncPeerConfig{
			Name:     m.Name,
			BaseURL:  url,
			Secret:   "",
			Fallback: m.Fallback,
		})
	}
	return peers
}

func (s *PanelSyncService) migratePeersToMembersIfNeeded(cfg *PanelSyncConfig, members []ClusterMember) []ClusterMember {
	if len(members) > 0 || len(cfg.Peers) == 0 {
		return members
	}
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
	return members
}

func remoteMemberDisplayName(m ClusterMember) string {
	name := strings.TrimSpace(m.Name)
	if name != "" && name != "本机" {
		return name
	}
	if id := strings.TrimSpace(m.NodeId); id != "" {
		return id
	}
	return "节点"
}

// filterImportedMembers 去掉种子/对端返回的「本机」记录，避免在本机成员表里显示成对端地址却标为本机
func filterImportedMembers(members []ClusterMember, remoteSelfNodeId, remoteBaseURL string) []ClusterMember {
	remoteURL := normalizePeerKey(remoteBaseURL)
	remoteID := strings.TrimSpace(remoteSelfNodeId)
	out := make([]ClusterMember, 0, len(members))
	for _, m := range members {
		url := normalizePeerKey(m.PublicURL)
		id := strings.TrimSpace(m.NodeId)
		if remoteID != "" && id == remoteID {
			continue
		}
		if remoteURL != "" && url == remoteURL {
			continue
		}
		if strings.TrimSpace(m.Name) == "本机" {
			m.Name = remoteMemberDisplayName(m)
		}
		out = append(out, m)
	}
	return out
}

func hasMislabeledLocalMembers(members []ClusterMember, cfg *PanelSyncConfig) bool {
	selfURL := normalizePeerKey(cfg.PublicURL)
	for _, m := range members {
		if strings.TrimSpace(m.Name) == "本机" && normalizePeerKey(m.PublicURL) != selfURL {
			return true
		}
	}
	return false
}

func (s *PanelSyncService) sanitizeClusterMembers(cfg *PanelSyncConfig, members []ClusterMember) []ClusterMember {
	selfURL := normalizePeerKey(cfg.PublicURL)
	selfID := strings.TrimSpace(cfg.NodeId)
	cleaned := make([]ClusterMember, 0, len(members))
	for _, m := range members {
		url := normalizePeerKey(m.PublicURL)
		id := strings.TrimSpace(m.NodeId)
		isSelf := (selfID != "" && id == selfID) || (selfURL != "" && url == selfURL)
		if isSelf {
			continue
		}
		if strings.TrimSpace(m.Name) == "本机" {
			if url == "" || url == selfURL {
				continue
			}
			m.Name = remoteMemberDisplayName(m)
		}
		cleaned = append(cleaned, m)
	}
	return s.ensureSelfMember(cfg, cleaned)
}

func (s *PanelSyncService) ensureSelfMember(cfg *PanelSyncConfig, members []ClusterMember) []ClusterMember {
	selfURL := normalizePeerKey(cfg.PublicURL)
	if selfURL == "" || strings.TrimSpace(cfg.NodeId) == "" {
		return members
	}
	self := ClusterMember{
		NodeId:    cfg.NodeId,
		PublicURL: selfURL,
		Name:      "本机",
		Fallback:  cfg.LocalFallback,
		UpdatedAt: time.Now().UnixMilli(),
	}
	return mergeClusterMembers(members, []ClusterMember{self})
}

func (s *PanelSyncService) rebuildPeersFromMembers(cfg *PanelSyncConfig, members []ClusterMember) error {
	peers := s.membersToPeers(cfg, members)
	raw, err := json.Marshal(peers)
	if err != nil {
		return err
	}
	cfg.Peers = peers
	return s.settingService.setString("panelSyncPeers", string(raw))
}

func (s *PanelSyncService) ListClusterMembers() (*ClusterMembersResponse, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	members, err := s.loadClusterMembers()
	if err != nil {
		return nil, err
	}
	members = s.sanitizeClusterMembers(cfg, members)
	return &ClusterMembersResponse{
		SelfNodeId: cfg.NodeId,
		Members:    members,
	}, nil
}

func (s *PanelSyncService) announceSelfMember(cfg *PanelSyncConfig) error {
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	selfURL := normalizePeerKey(cfg.PublicURL)
	if selfURL == "" {
		return nil
	}
	return s.Emit(SyncEventClusterMemberUpsert, clusterMemberPayload{
		NodeId:    cfg.NodeId,
		PublicURL: selfURL,
		Name:      "本机",
		Fallback:  cfg.LocalFallback,
		UpdatedAt: time.Now().UnixMilli(),
	})
}

func (s *PanelSyncService) FetchAndMergeMembersFromSeed(seedBaseURL string) (int, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return 0, err
	}
	if !cfg.Enabled {
		return 0, common.NewError("请先启用多机同步")
	}
	seed := strings.TrimSpace(seedBaseURL)
	if seed == "" {
		return 0, common.NewError("种子节点地址为空")
	}
	seed = normalizePeerKey(seed)
	remoteRes, err := fetchPeerMembers(SyncPeerConfig{BaseURL: seed}, cfg.Secret)
	if err != nil {
		return 0, common.NewError("拉取站群成员失败:", err)
	}
	remote := filterImportedMembers(remoteRes.Members, remoteRes.SelfNodeId, seed)
	seedMember := ClusterMember{PublicURL: seed, Name: "种子节点", UpdatedAt: time.Now().UnixMilli()}
	local, _ := s.loadClusterMembers()
	local = s.migratePeersToMembersIfNeeded(cfg, local)
	before := len(local)
	local = mergeClusterMembers(local, remote)
	local = mergeClusterMembers(local, []ClusterMember{seedMember})
	local = s.sanitizeClusterMembers(cfg, local)
	if err := s.saveClusterMembers(local); err != nil {
		return 0, err
	}
	if err := s.rebuildPeersFromMembers(cfg, local); err != nil {
		return 0, err
	}
	_ = s.announceSelfMember(cfg)
	return len(local) - before, nil
}

func (s *PanelSyncService) RemoveClusterMember(nodeId, publicURL string) error {
	cfg, err := s.GetConfig()
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		return common.NewError("请先启用多机同步")
	}
	targetID := strings.TrimSpace(nodeId)
	targetURL := normalizePeerKey(publicURL)
	if targetID == "" && targetURL == "" {
		return common.NewError("请指定要移除的节点")
	}
	if targetID == cfg.NodeId || targetURL == normalizePeerKey(cfg.PublicURL) {
		return common.NewError("不能移出本机")
	}
	members, _ := s.loadClusterMembers()
	next := make([]ClusterMember, 0, len(members))
	var removed ClusterMember
	found := false
	for _, m := range members {
		match := (targetID != "" && strings.TrimSpace(m.NodeId) == targetID) ||
			(targetURL != "" && normalizePeerKey(m.PublicURL) == targetURL)
		if match {
			removed = m
			found = true
			continue
		}
		next = append(next, m)
	}
	if !found {
		return common.NewError("站群成员不存在")
	}
	if err := s.saveClusterMembers(next); err != nil {
		return err
	}
	if err := s.rebuildPeersFromMembers(cfg, next); err != nil {
		return err
	}
	peerKey := normalizePeerKey(removed.PublicURL)
	if peerKey != "" {
		db := database.GetDB()
		_ = db.Where("peer_key = ?", peerKey).Delete(&model.SyncPeerCursor{}).Error
	}
	payload := clusterMemberRemovePayload{
		NodeId:    removed.NodeId,
		PublicURL: peerKey,
	}
	if s.IsApplying() {
		return nil
	}
	return s.Emit(SyncEventClusterMemberRemove, payload)
}

func (s *PanelSyncService) applyClusterMemberUpsert(raw json.RawMessage) error {
	var p clusterMemberPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	cfg, err := s.GetConfig()
	if err != nil {
		return err
	}
	members, _ := s.loadClusterMembers()
	incoming := ClusterMember{
		NodeId:    p.NodeId,
		PublicURL: p.PublicURL,
		Name:      p.Name,
		Fallback:  p.Fallback,
		UpdatedAt: p.UpdatedAt,
	}
	selfURL := normalizePeerKey(cfg.PublicURL)
	if incoming.NodeId != cfg.NodeId && normalizePeerKey(incoming.PublicURL) != selfURL {
		if strings.TrimSpace(incoming.Name) == "本机" {
			incoming.Name = remoteMemberDisplayName(incoming)
		}
	}
	members = mergeClusterMembers(members, []ClusterMember{incoming})
	members = s.sanitizeClusterMembers(cfg, members)
	if err := s.saveClusterMembers(members); err != nil {
		return err
	}
	return s.rebuildPeersFromMembers(cfg, members)
}

func (s *PanelSyncService) applyClusterMemberRemove(raw json.RawMessage) error {
	var p clusterMemberRemovePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	cfg, err := s.GetConfig()
	if err != nil {
		return err
	}
	targetID := strings.TrimSpace(p.NodeId)
	targetURL := normalizePeerKey(p.PublicURL)
	if targetID == cfg.NodeId || targetURL == normalizePeerKey(cfg.PublicURL) {
		return nil
	}
	members, _ := s.loadClusterMembers()
	next := make([]ClusterMember, 0, len(members))
	for _, m := range members {
		match := (targetID != "" && strings.TrimSpace(m.NodeId) == targetID) ||
			(targetURL != "" && normalizePeerKey(m.PublicURL) == targetURL)
		if match {
			continue
		}
		next = append(next, m)
	}
	next = s.sanitizeClusterMembers(cfg, next)
	if err := s.saveClusterMembers(next); err != nil {
		return err
	}
	if targetURL != "" {
		db := database.GetDB()
		_ = db.Where("peer_key = ?", targetURL).Delete(&model.SyncPeerCursor{}).Error
	}
	return s.rebuildPeersFromMembers(cfg, next)
}

func (s *PanelSyncService) persistMembersAndPeers(cfg *PanelSyncConfig, members []ClusterMember, announce bool) error {
	members = s.migratePeersToMembersIfNeeded(cfg, members)
	members = s.sanitizeClusterMembers(cfg, members)
	if err := s.saveClusterMembers(members); err != nil {
		return err
	}
	if err := s.rebuildPeersFromMembers(cfg, members); err != nil {
		return err
	}
	if announce && cfg.Enabled {
		return s.announceSelfMember(cfg)
	}
	return nil
}
