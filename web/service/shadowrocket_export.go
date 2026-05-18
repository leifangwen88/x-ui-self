package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"x-ui/database/model"
)

func srSanitizeName(s string) string {
	s = strings.ReplaceAll(s, ",", " ")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return "node"
	}
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

func srEscapeGroupName(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), ",", "，")
}

func inboundToShadowrocketProxy(name string, ib *model.Inbound, connectHost string) (string, bool) {
	if ib == nil || !ib.Enable || !InboundSupportsLink(ib.Protocol) {
		return "", false
	}
	name = srSanitizeName(name)
	server := strings.TrimSpace(connectHost)
	if server == "" {
		server = ResolveInboundAddress(ib, "", connectHost)
	}
	port := strconv.Itoa(ib.Port)
	st := parseStreamSettings(ib.StreamSettings)

	switch ib.Protocol {
	case model.VMess:
		return buildSRVmess(name, server, port, ib, st)
	case model.VLESS:
		return buildSRVless(name, server, port, ib, st)
	case model.Trojan:
		return buildSRTrojan(name, server, port, ib, st)
	case model.Shadowsocks:
		return buildSRShadowsocks(name, server, port, ib)
	default:
		return "", false
	}
}

func joinSRProxy(name, proto string, server, port string, params ...string) string {
	parts := []string{srSanitizeName(name) + " = " + proto, server, port}
	parts = append(parts, params...)
	return strings.Join(parts, ",")
}

func appendStreamParams(params []string, st *streamSettings) []string {
	sni := ""
	if st.TLSSettings != nil {
		sni = st.TLSSettings.ServerName
	}
	if sni == "" && st.XTlsSettings != nil {
		sni = st.XTlsSettings.ServerName
	}
	switch st.Network {
	case "ws":
		params = append(params, "obfs=websocket")
		if st.WSSettings != nil {
			if p := strings.TrimSpace(st.WSSettings.Path); p != "" {
				params = append(params, "obfs-uri="+p)
			}
			if h := headerValue(st.WSSettings.Headers, "host"); h != "" {
				params = append(params, "obfs-host="+h)
			}
		}
	case "grpc":
		params = append(params, "obfs=grpc")
		if st.GRPCSettings != nil {
			if sn := strings.TrimSpace(st.GRPCSettings.ServiceName); sn != "" {
				params = append(params, "obfs-uri="+sn)
			}
		}
	case "http", "h2":
		params = append(params, "obfs=http")
		if st.HTTPSettings != nil && len(st.HTTPSettings.Path) > 0 {
			params = append(params, "obfs-uri="+st.HTTPSettings.Path[0])
		}
		if st.HTTPSettings != nil && len(st.HTTPSettings.Host) > 0 {
			params = append(params, "obfs-host="+st.HTTPSettings.Host[0])
		}
	}
	if st.Security == "tls" || st.Security == "xtls" {
		params = append(params, "tls=true")
		if sni != "" {
			params = append(params, "peer="+sni)
		}
		if st.Security == "xtls" {
			params = append(params, "xtls=true")
		}
	}
	return params
}

func buildSRVmess(name, server, port string, ib *model.Inbound, st *streamSettings) (string, bool) {
	var settings struct {
		Clients []struct {
			ID      string `json:"id"`
			AlterID int    `json:"alterId"`
		} `json:"clients"`
	}
	if json.Unmarshal([]byte(ib.Settings), &settings) != nil || len(settings.Clients) == 0 {
		return "", false
	}
	c := settings.Clients[0]
	params := []string{
		"password=" + c.ID,
		"method=auto",
	}
	if c.AlterID > 0 {
		params = append(params, "alterId="+strconv.Itoa(c.AlterID))
	}
	params = appendStreamParams(params, st)
	return joinSRProxy(name, "vmess", server, port, params...), true
}

func buildSRVless(name, server, port string, ib *model.Inbound, st *streamSettings) (string, bool) {
	var settings struct {
		Clients []struct {
			ID   string `json:"id"`
			Flow string `json:"flow"`
		} `json:"clients"`
	}
	if json.Unmarshal([]byte(ib.Settings), &settings) != nil || len(settings.Clients) == 0 {
		return "", false
	}
	c := settings.Clients[0]
	params := []string{"password=" + c.ID}
	if st.Security == "xtls" && c.Flow != "" {
		params = append(params, "flow="+c.Flow)
	}
	params = appendStreamParams(params, st)
	return joinSRProxy(name, "vless", server, port, params...), true
}

func buildSRTrojan(name, server, port string, ib *model.Inbound, st *streamSettings) (string, bool) {
	var settings struct {
		Clients []struct {
			Password string `json:"password"`
		} `json:"clients"`
	}
	if json.Unmarshal([]byte(ib.Settings), &settings) != nil || len(settings.Clients) == 0 {
		return "", false
	}
	params := []string{"password=" + settings.Clients[0].Password}
	params = appendStreamParams(params, st)
	return joinSRProxy(name, "trojan", server, port, params...), true
}

func buildSRShadowsocks(name, server, port string, ib *model.Inbound) (string, bool) {
	var settings struct {
		Method   string `json:"method"`
		Password string `json:"password"`
	}
	if json.Unmarshal([]byte(ib.Settings), &settings) != nil {
		return "", false
	}
	params := []string{
		"password=" + settings.Password,
		"method=" + settings.Method,
	}
	return joinSRProxy(name, "ss", server, port, params...), true
}

func srHealthSuffix() string {
	return "url=http://www.gstatic.com/generate_204,interval=60,timeout=5,tolerance=50"
}

func buildClusterSRInboundGroups(groupName string, primary, fallback []string) []string {
	gn := srEscapeGroupName(groupName)
	if len(primary) == 0 && len(fallback) == 0 {
		return nil
	}
	suffix := srHealthSuffix()
	var lines []string
	primaryPick := gn + "-首选"
	if len(primary) == 0 {
		lines = append(lines, fmt.Sprintf("%s = fallback,%s,%s", gn, strings.Join(fallback, ","), suffix))
		return lines
	}
	if len(fallback) == 0 {
		if len(primary) == 1 {
			return nil
		}
		lines = append(lines, fmt.Sprintf("%s = load-balance,%s,%s", gn, strings.Join(primary, ","), suffix))
		return lines
	}
	chain := make([]string, 0, 1+len(fallback))
	if len(primary) == 1 {
		chain = append(chain, primary[0])
	} else {
		lines = append(lines, fmt.Sprintf("%s = load-balance,%s,%s", primaryPick, strings.Join(primary, ","), suffix))
		chain = append(chain, primaryPick)
	}
	chain = append(chain, fallback...)
	lines = append(lines, fmt.Sprintf("%s = fallback,%s,%s", gn, strings.Join(chain, ","), suffix))
	return lines
}

func genClusterShadowrocketConf(members map[string][]clusterMember, order []string, games []*model.Game) string {
	_, groups := buildClusterInboundGroups(members, order, games)
	if len(groups) == 0 {
		return ""
	}
	proxyLines := make([]string, 0)
	nameSet := make(map[string]bool)
	for _, key := range order {
		list := dedupeClusterMembersByHost(members[key])
		if len(list) == 0 {
			continue
		}
		displayName := displayInboundName(list[0].Inbound.Remark, list[0].Inbound.Port)
		for _, m := range list {
			pname := displayName
			if len(list) > 1 {
				pname = fmt.Sprintf("%s @ %s", displayName, clusterSubDisplayLabel(m.NodeLabel, m.ConnectHost))
			}
			pname = srSanitizeName(pname)
			if nameSet[pname] {
				continue
			}
			line, ok := inboundToShadowrocketProxy(pname, m.Inbound, m.ConnectHost)
			if !ok {
				continue
			}
			nameSet[pname] = true
			proxyLines = append(proxyLines, line)
		}
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

	var groupLines []string
	var rootGames []string
	for _, gid := range gameOrder {
		inbounds := byGame[gid]
		if len(inbounds) == 0 {
			continue
		}
		gn := srEscapeGroupName(clusterGameName(gid, gameName))
		var inboundRoots []string
		for _, ig := range inbounds {
			primary := make([]string, len(ig.Primary))
			for i, n := range ig.Primary {
				primary[i] = srSanitizeName(n)
			}
			fallback := make([]string, len(ig.Fallback))
			for i, n := range ig.Fallback {
				fallback[i] = srSanitizeName(n)
			}
			blocks := buildClusterSRInboundGroups(ig.GroupName, primary, fallback)
			if len(blocks) == 0 {
				if len(primary) == 1 {
					inboundRoots = append(inboundRoots, primary[0])
				}
				continue
			}
			inboundRoots = append(inboundRoots, srEscapeGroupName(ig.GroupName))
			groupLines = append(groupLines, blocks...)
		}
		if len(inboundRoots) == 0 {
			continue
		}
		rootGames = append(rootGames, gn)
		groupLines = append(groupLines, fmt.Sprintf("%s = select,%s,DIRECT", gn, strings.Join(inboundRoots, ",")))
	}

	var b strings.Builder
	b.WriteString("# Shadowrocket 站群订阅（按游戏→入站备注，入站内 load-balance，兜底机 fallback）\n")
	b.WriteString("# 首页添加订阅或：配置 → 下载配置 → 粘贴本 URL\n\n")
	b.WriteString("[General]\n")
	b.WriteString("bypass-system = true\n")
	b.WriteString("dns-server = system\n")
	b.WriteString("ipv6 = false\n\n")
	b.WriteString("[Proxy]\n")
	b.WriteString(strings.Join(proxyLines, "\n"))
	b.WriteString("\n\n[Proxy Group]\n")
	b.WriteString(strings.Join(groupLines, "\n"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("%s = select,%s,DIRECT\n\n", srEscapeGroupName(ClashGroupClusterLB), strings.Join(rootGames, ",")))
	b.WriteString("[Rule]\n")
	b.WriteString("FINAL,")
	b.WriteString(srEscapeGroupName(ClashGroupClusterLB))
	b.WriteString("\n")
	return b.String()
}
