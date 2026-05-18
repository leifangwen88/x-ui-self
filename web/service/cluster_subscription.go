package service

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"x-ui/database/model"
)

type clusterMember struct {
	NodeLabel   string
	Inbound     *model.Inbound
	ConnectHost string // 节点对外 IP，从对等 baseUrl 或本机访问 IP 解析
	GameId      int    // 绑定游戏（对端由 gameCode 解析）
	Fallback    bool   // 兜底节点（高价稳定，仅首选不可用时启用）
}

type clusterInboundGroup struct {
	GameId      int
	DisplayName string
	GroupName   string
	Primary     []string
	Fallback    []string
}

type clusterSlot struct {
	gameId int
}

// ClusterSubMeta 站群订阅在面板中的展示信息
type ClusterSubMeta struct {
	Enabled      bool   `json:"enabled"`
	PeerCount    int    `json:"peerCount"`
	AlignedCount int    `json:"alignedCount"`
	ClashPath        string `json:"clashPath"`
	ShadowrocketPath string `json:"shadowrocketPath"`
	Base64Path       string `json:"base64Path"`
	LinksPath        string `json:"linksPath"`
	Hint         string `json:"hint,omitempty"`
}

func clusterInboundKey(remark string, port int) string {
	r := strings.TrimSpace(remark)
	if r != "" {
		return "remark:" + r
	}
	return fmt.Sprintf("port:%d", port)
}

func displayInboundName(remark string, port int) string {
	r := strings.TrimSpace(remark)
	if r != "" {
		return r
	}
	return fmt.Sprintf("inbound-%d", port)
}

// peerConnectHost 从对等面板地址 http://IP:端口/路径 解析节点 IP（无域名场景）
func peerConnectHost(peer SyncPeerConfig) string {
	raw := strings.TrimSpace(peer.BaseURL)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return hostOnly(raw)
	}
	if h := u.Hostname(); h != "" {
		return h
	}
	return hostOnly(u.Host)
}

func inboundBackupToInbound(ib InboundBackup) *model.Inbound {
	policy := ib.RotationPolicy
	if policy == "" {
		policy = model.RotationPolicyPreferUnusedUnbanned
	}
	return &model.Inbound{
		Remark:         ib.Remark,
		Enable:         ib.Enable,
		ExpiryTime:     ib.ExpiryTime,
		Listen:         ib.Listen,
		Port:           ib.Port,
		Protocol:       model.Protocol(ib.Protocol),
		Settings:       ib.Settings,
		StreamSettings: ib.StreamSettings,
		Sniffing:       ib.Sniffing,
		RotationEnable: ib.RotationEnable,
		RotationPolicy: policy,
		LastRotatedAt:  ib.LastRotatedAt,
	}
}

func normalizeClusterGameId(gameId int) int {
	if gameId <= 0 {
		return 0
	}
	return gameId
}

func buildGameCodeToId(games []*model.Game) map[string]int {
	m := make(map[string]int)
	for _, g := range games {
		if g == nil {
			continue
		}
		code := strings.TrimSpace(g.Code)
		if code != "" {
			m[code] = g.Id
		}
	}
	return m
}

func clusterGameName(gameId int, gameName map[int]string) string {
	gn := gameName[gameId]
	if gn != "" {
		return gn
	}
	if gameId == 0 {
		return "未指定游戏"
	}
	return fmt.Sprintf("游戏#%d", gameId)
}

func clusterGameOrder(byGame map[int][]clusterSlot, games []*model.Game) []int {
	order := make([]int, 0)
	seen := make(map[int]bool)
	for _, g := range games {
		if g == nil || len(byGame[g.Id]) == 0 {
			continue
		}
		order = append(order, g.Id)
		seen[g.Id] = true
	}
	for gid := range byGame {
		if gid == 0 || seen[gid] || len(byGame[gid]) == 0 {
			continue
		}
		order = append(order, gid)
	}
	if len(byGame[0]) > 0 {
		order = append(order, 0)
	}
	return order
}

func inboundMatchesGameFilter(ib *model.Inbound, gameId int) bool {
	if ib == nil || !ib.Enable || !InboundSupportsLink(ib.Protocol) {
		return false
	}
	if gameId < 0 {
		return true
	}
	gid := ib.GameId
	if gid <= 0 {
		gid = 0
	}
	return gid == gameId
}

func (s *SubscriptionService) GetClusterSubMeta() (*ClusterSubMeta, error) {
	token, err := s.settingService.GetSubToken()
	if err != nil {
		return nil, err
	}
	meta := &ClusterSubMeta{Enabled: false}
	if globalPanelSync == nil {
		meta.Hint = "多机同步未初始化"
		return meta, nil
	}
	cfg, err := globalPanelSync.GetConfig()
	if err != nil || cfg == nil || !cfg.Enabled || len(cfg.Peers) == 0 {
		meta.Hint = "请先在设置中启用多机同步并配置对等节点"
		return meta, nil
	}
	meta.Enabled = true
	meta.PeerCount = len(cfg.Peers)
	for _, st := range cfg.PeerStatus {
		if st.AlignedAt > 0 {
			meta.AlignedCount++
		}
	}
	if meta.AlignedCount == 0 {
		meta.Hint = "建议先完成首次对齐向导后再使用站群订阅"
	} else {
		meta.Hint = "站群按入站备注聚合（同备注跨机合并为入口1等）；同步/导入按端口对齐，请保证各机同端口备注一致"
	}
	prefix := "sub/" + token
	meta.ShadowrocketPath = prefix + "?type=cluster-shadowrocket"
	meta.ClashPath = prefix + "?type=cluster-clash"
	meta.Base64Path = prefix + "?type=cluster"
	meta.LinksPath = prefix + "?type=cluster&format=links"
	return meta, nil
}

func (s *SubscriptionService) collectClusterMembers(subHost string, localRequestHost string, gameId int) (map[string][]clusterMember, []string, error) {
	members := make(map[string][]clusterMember)
	order := make([]string, 0)
	seenOrder := make(map[string]bool)

	addMember := func(key string, m clusterMember) {
		if _, ok := members[key]; !ok {
			order = append(order, key)
			seenOrder[key] = true
		}
		members[key] = append(members, m)
	}

	localLabel := "本机"
	localFallback := false
	if globalPanelSync != nil {
		if cfg, err := globalPanelSync.GetConfig(); err == nil {
			if strings.TrimSpace(cfg.NodeId) != "" {
				localLabel = strings.TrimSpace(cfg.NodeId)
			}
			localFallback = cfg.LocalFallback
		}
	}
	localInbounds, err := s.filterInbounds(gameId)
	if err != nil {
		return nil, nil, err
	}
	localHost := hostOnly(localRequestHost)
	for _, ib := range localInbounds {
		key := clusterInboundKey(ib.Remark, ib.Port)
		addMember(key, clusterMember{
			NodeLabel: localLabel, Inbound: ib, ConnectHost: localHost,
			GameId: normalizeClusterGameId(ib.GameId), Fallback: localFallback,
		})
	}

	if globalPanelSync == nil {
		return members, order, nil
	}
	cfg, err := globalPanelSync.GetConfig()
	if err != nil || !cfg.Enabled {
		return members, order, nil
	}
	games, _ := s.gameService.GetAll()
	gameCodeToId := buildGameCodeToId(games)
	secret := strings.TrimSpace(cfg.Secret)
	for _, peer := range cfg.Peers {
		label := strings.TrimSpace(peer.Name)
		if label == "" {
			label = peerConnectHost(peer)
		}
		if label == "" {
			label = "peer"
		}
		snap, err := fetchPeerSnapshot(peer, secret)
		if err != nil || snap == nil {
			continue
		}
		connectHost := peerConnectHost(peer)
		if connectHost == "" {
			continue
		}
		wantGameCode := ""
		if gameId > 0 {
			if g, err := s.gameService.GetById(gameId); err == nil {
				wantGameCode = strings.TrimSpace(g.Code)
			}
		}
		for _, ibBak := range snap.Inbounds {
			ib := inboundBackupToInbound(ibBak)
			gc := strings.TrimSpace(ibBak.GameCode)
			if gameId >= 0 {
				if gameId == 0 {
					if gc != "" {
						continue
					}
				} else if gc != wantGameCode {
					continue
				}
			}
			if !ib.Enable || !InboundSupportsLink(ib.Protocol) {
				continue
			}
			memberGameId := 0
			if gc != "" {
				if id, ok := gameCodeToId[gc]; ok {
					memberGameId = id
				}
			}
			key := clusterInboundKey(ib.Remark, ib.Port)
			addMember(key, clusterMember{
				NodeLabel: label, Inbound: ib, ConnectHost: connectHost,
				GameId: memberGameId, Fallback: peer.Fallback,
			})
		}
	}
	return members, order, nil
}

func clusterInboundGroupName(gameId int, gameName, inboundDisplay string, used map[string]bool) string {
	base := strings.TrimSpace(inboundDisplay)
	if base == "" {
		base = "inbound"
	}
	if !used[base] {
		used[base] = true
		return base
	}
	name := strings.TrimSpace(gameName) + "·" + base
	if !used[name] {
		used[name] = true
		return name
	}
	name = fmt.Sprintf("%s·%d", name, gameId)
	used[name] = true
	return name
}

func clusterGameOrderFromInboundGroups(groups []clusterInboundGroup, games []*model.Game) []int {
	bySlot := make(map[int][]clusterSlot)
	for _, g := range groups {
		if len(g.Primary)+len(g.Fallback) == 0 {
			continue
		}
		bySlot[g.GameId] = append(bySlot[g.GameId], clusterSlot{gameId: g.GameId})
	}
	return clusterGameOrder(bySlot, games)
}

func buildClusterInboundGroups(members map[string][]clusterMember, order []string, games []*model.Game) ([]string, []clusterInboundGroup) {
	proxyBlocks := make([]string, 0)
	groups := make([]clusterInboundGroup, 0)
	usedGroupNames := make(map[string]bool)
	gameName := make(map[int]string)
	for _, g := range games {
		if g != nil {
			gameName[g.Id] = g.Name
		}
	}

	for _, key := range order {
		list := members[key]
		if len(list) == 0 {
			continue
		}
		displayName := displayInboundName(list[0].Inbound.Remark, list[0].Inbound.Port)
		gid := normalizeClusterGameId(list[0].GameId)
		ig := clusterInboundGroup{
			GameId:      gid,
			DisplayName: displayName,
			GroupName:   clusterInboundGroupName(gid, clusterGameName(gid, gameName), displayName, usedGroupNames),
		}
		for _, m := range list {
			pname := displayName
			if len(list) > 1 {
				pname = fmt.Sprintf("%s @ %s", displayName, m.NodeLabel)
			}
			lines, pname, ok := inboundToClashLinesWithName(m.Inbound, "", m.ConnectHost, pname)
			if !ok {
				continue
			}
			block := "  -"
			for _, line := range lines {
				block += "\n    " + line
			}
			proxyBlocks = append(proxyBlocks, block)
			if m.Fallback {
				ig.Fallback = append(ig.Fallback, pname)
			} else {
				ig.Primary = append(ig.Primary, pname)
			}
		}
		if len(ig.Primary)+len(ig.Fallback) > 0 {
			groups = append(groups, ig)
		}
	}
	return proxyBlocks, groups
}

func (s *SubscriptionService) GenClusterLinksText(subHost string, requestHost string, gameId int) string {
	members, order, err := s.collectClusterMembers(subHost, requestHost, gameId)
	if err != nil || len(order) == 0 {
		return ""
	}
	games, _ := s.gameService.GetAll()
	gameName := make(map[int]string)
	for _, g := range games {
		if g != nil {
			gameName[g.Id] = g.Name
		}
	}
	_, groups := buildClusterInboundGroups(members, order, games)
	byGameOrder := make(map[int][]clusterInboundGroup)
	for _, ig := range groups {
		byGameOrder[ig.GameId] = append(byGameOrder[ig.GameId], ig)
	}
	gameOrder := clusterGameOrderFromInboundGroups(groups, games)
	var lines []string
	for _, gid := range gameOrder {
		inbounds := byGameOrder[gid]
		if len(inbounds) == 0 {
			continue
		}
		if len(gameOrder) > 1 {
			lines = append(lines, "# "+clusterGameName(gid, gameName))
		}
		for _, ig := range inbounds {
			lines = append(lines, "# "+ig.GroupName)
			for _, key := range order {
				list := members[key]
				if len(list) == 0 {
					continue
				}
				if displayInboundName(list[0].Inbound.Remark, list[0].Inbound.Port) != ig.DisplayName {
					continue
				}
				if normalizeClusterGameId(list[0].GameId) != gid {
					continue
				}
				for _, m := range list {
					link := GenInboundShareLink(m.Inbound, "", m.ConnectHost)
					if link != "" {
						lines = append(lines, link)
					}
				}
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (s *SubscriptionService) GenClusterBase64Subscription(subHost string, requestHost string, gameId int) string {
	text := s.GenClusterLinksText(subHost, requestHost, gameId)
	if text == "" {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(text))
}

func (s *SubscriptionService) ClusterSubEnabled() bool {
	meta, err := s.GetClusterSubMeta()
	return err == nil && meta != nil && meta.Enabled
}

func (s *SubscriptionService) GenClusterClashSubscription(subHost string, requestHost string, gameId int) string {
	members, order, err := s.collectClusterMembers(subHost, requestHost, gameId)
	if err != nil || len(order) == 0 {
		return ""
	}
	games, _ := s.gameService.GetAll()
	return genClusterClashYamlByGame(members, order, games)
}

func (s *SubscriptionService) GenClusterShadowrocketSubscription(subHost string, requestHost string, gameId int) string {
	members, order, err := s.collectClusterMembers(subHost, requestHost, gameId)
	if err != nil || len(order) == 0 {
		return ""
	}
	games, _ := s.gameService.GetAll()
	return genClusterShadowrocketConf(members, order, games)
}

func genClusterClashYamlByGame(members map[string][]clusterMember, order []string, games []*model.Game) string {
	proxyBlocks, groups := buildClusterInboundGroups(members, order, games)
	if len(proxyBlocks) == 0 {
		return ""
	}
	gameName := make(map[int]string)
	for _, g := range games {
		if g != nil {
			gameName[g.Id] = g.Name
		}
	}
	byGame := make(map[int][]clusterInboundGroup)
	for _, ig := range groups {
		byGame[ig.GameId] = append(byGame[ig.GameId], ig)
	}
	gameOrder := clusterGameOrderFromInboundGroups(groups, games)
	if len(gameOrder) == 0 {
		return ""
	}

	var groupBlocks []string
	var rootGames []string
	for _, gid := range gameOrder {
		inbounds := byGame[gid]
		if len(inbounds) == 0 {
			continue
		}
		gn := clusterGameName(gid, gameName)
		var inboundRoots []string
		for _, ig := range inbounds {
			blocks, root := buildClusterInboundGroupYamls(ig.GroupName, ig.Primary, ig.Fallback)
			if root == "" {
				continue
			}
			groupBlocks = append(groupBlocks, blocks...)
			inboundRoots = append(inboundRoots, root)
		}
		if len(inboundRoots) == 0 {
			continue
		}
		groupBlocks = append(groupBlocks, clashSelectGroupYaml(gn, inboundRoots))
		rootGames = append(rootGames, yamlQuote(gn))
	}

	var b strings.Builder
	b.WriteString("# Clash / Mihomo 站群订阅（按游戏→入站备注，入站内 load-balance，兜底机 fallback）\n")
	b.WriteString("proxies:\n")
	b.WriteString(strings.Join(proxyBlocks, "\n"))
	b.WriteString("\n\nproxy-groups:\n")
	b.WriteString(strings.Join(groupBlocks, ""))
	b.WriteString(fmt.Sprintf("  - name: %s\n    type: select\n    proxies:\n", yamlQuote(ClashGroupClusterLB)))
	for _, g := range rootGames {
		b.WriteString("      - " + g + "\n")
	}
	b.WriteString("      - DIRECT\n\nrules:\n  - MATCH,")
	b.WriteString(ClashGroupClusterLB)
	b.WriteString("\n")
	return b.String()
}

func inboundToClashLinesWithName(ib *model.Inbound, subHost, requestHost, proxyName string) ([]string, string, bool) {
	lines, _, ok := inboundToClashLines(ib, subHost, requestHost)
	if !ok {
		return nil, "", false
	}
	if strings.TrimSpace(proxyName) == "" {
		proxyName = clashProxyName(ib)
	}
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "name:") {
			lines[i] = "name: " + yamlQuote(proxyName)
			break
		}
	}
	return lines, proxyName, true
}
