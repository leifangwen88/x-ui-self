package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"x-ui/database/model"
)

// Clash 订阅在客户端展示的顶层策略组名称
const (
	ClashGroupSingleSite = "单站"
	ClashGroupClusterLB  = "站群负载均衡"
)

func GenClashYamlByGame(inbounds []*model.Inbound, subHost string, requestHost string) string {
	gameName := make(map[int]string)
	gameService := GameService{}
	games, _ := gameService.GetAll()
	for _, g := range games {
		gameName[g.Id] = g.Name
	}
	byGame := make(map[int][]*model.Inbound)
	for _, ib := range inbounds {
		if ib == nil || !ib.Enable || !InboundSupportsLink(ib.Protocol) {
			continue
		}
		gid := ib.GameId
		if gid <= 0 {
			gid = 0
		}
		byGame[gid] = append(byGame[gid], ib)
	}
	order := clashGameOrder(byGame, games)
	if len(order) == 0 {
		return ""
	}
	var proxyBlocks []string
	var allNames []string
	groupBlocks := make([]string, 0)
	for _, gid := range order {
		list := byGame[gid]
		gn := gameName[gid]
		if gn == "" {
			if gid == 0 {
				gn = "未指定游戏"
			} else {
				gn = fmt.Sprintf("游戏#%d", gid)
			}
		}
		var names []string
		for _, ib := range list {
			lines, name, ok := inboundToClashLines(ib, subHost, requestHost)
			if !ok {
				continue
			}
			names = append(names, name)
			allNames = append(allNames, name)
			block := "  -"
			for _, line := range lines {
				block += "\n    " + line
			}
			proxyBlocks = append(proxyBlocks, block)
		}
		if len(names) > 0 {
			groupBlocks = append(groupBlocks, fmt.Sprintf("  - name: %s\n    type: select\n    proxies:", yamlQuote(gn)))
			for _, n := range names {
				groupBlocks = append(groupBlocks, "      - "+yamlQuote(n))
			}
			groupBlocks = append(groupBlocks, "      - DIRECT")
		}
	}
	if len(allNames) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Clash / Mihomo 单站订阅（按游戏分组）\n")
	b.WriteString("proxies:\n")
	b.WriteString(strings.Join(proxyBlocks, "\n"))
	b.WriteString("\n\nproxy-groups:\n")
	b.WriteString(strings.Join(groupBlocks, "\n"))
	b.WriteString(fmt.Sprintf("\n  - name: %s\n    type: select\n    proxies:\n", yamlQuote(ClashGroupSingleSite)))
	for _, gid := range order {
		gn := gameName[gid]
		if gn == "" && gid == 0 {
			gn = "未指定游戏"
		} else if gn == "" {
			gn = fmt.Sprintf("游戏#%d", gid)
		}
		if len(byGame[gid]) > 0 {
			b.WriteString("      - ")
			b.WriteString(yamlQuote(gn))
			b.WriteString("\n")
		}
	}
	b.WriteString("      - DIRECT\n\nrules:\n  - MATCH,")
	b.WriteString(ClashGroupSingleSite)
	b.WriteString("\n")
	return b.String()
}

func clashGameOrder(byGame map[int][]*model.Inbound, games []*model.Game) []int {
	order := make([]int, 0)
	seen := make(map[int]bool)
	for _, g := range games {
		if len(byGame[g.Id]) == 0 {
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

func GenClashYaml(inbounds []*model.Inbound, subHost string, requestHost string) string {
	var proxyBlocks []string
	var names []string

	for _, ib := range inbounds {
		if ib == nil || !ib.Enable || !InboundSupportsLink(ib.Protocol) {
			continue
		}
		lines, name, ok := inboundToClashLines(ib, subHost, requestHost)
		if !ok {
			continue
		}
		names = append(names, name)
		block := "  -"
		for _, line := range lines {
			block += "\n    " + line
		}
		proxyBlocks = append(proxyBlocks, block)
	}

	if len(names) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("# Clash / Mihomo 单站订阅\n")
	b.WriteString("proxies:\n")
	b.WriteString(strings.Join(proxyBlocks, "\n"))
	b.WriteString("\n\nproxy-groups:\n")
	b.WriteString(fmt.Sprintf("  - name: %s\n", yamlQuote(ClashGroupSingleSite)))
	b.WriteString("    type: select\n")
	b.WriteString("    proxies:\n")
	for _, n := range names {
		b.WriteString("      - ")
		b.WriteString(yamlQuote(n))
		b.WriteString("\n")
	}
	b.WriteString("      - DIRECT\n\nrules:\n")
	b.WriteString("  - MATCH,")
	b.WriteString(ClashGroupSingleSite)
	b.WriteString("\n")
	return b.String()
}

const clusterHealthURL = "http://www.gstatic.com/generate_204"
const clusterHealthInterval = 300

// buildClusterInboundGroupYamls 单入站站群组：首选池 load-balance，整组 fallback 链接兜底机
func buildClusterInboundGroupYamls(groupName string, primary, fallback []string) (blocks []string, rootQuoted string) {
	gn := strings.TrimSpace(groupName)
	if gn == "" {
		return nil, ""
	}
	if len(primary) == 0 && len(fallback) == 0 {
		return nil, ""
	}
	primaryPick := gn + "-首选"
	if len(primary) == 0 {
		blocks = append(blocks, clashFallbackGroupYaml(gn, fallback))
		return blocks, yamlQuote(gn)
	}
	if len(fallback) == 0 {
		if len(primary) == 1 {
			return nil, yamlQuote(primary[0])
		}
		blocks = append(blocks, clashLoadBalanceGroupYaml(gn, primary))
		return blocks, yamlQuote(gn)
	}
	chain := make([]string, 0, 1+len(fallback))
	if len(primary) == 1 {
		chain = append(chain, primary[0])
	} else {
		blocks = append(blocks, clashLoadBalanceGroupYaml(primaryPick, primary))
		chain = append(chain, primaryPick)
	}
	chain = append(chain, fallback...)
	blocks = append(blocks, clashFallbackGroupYaml(gn, chain))
	return blocks, yamlQuote(gn)
}

func clashLoadBalanceGroupYaml(groupName string, proxyNames []string) string {
	if len(proxyNames) == 0 {
		return ""
	}
	var gb strings.Builder
	gb.WriteString(fmt.Sprintf("  - name: %s\n", yamlQuote(groupName)))
	gb.WriteString("    type: load-balance\n")
	gb.WriteString("    strategy: round-robin\n")
	gb.WriteString("    url: " + clusterHealthURL + "\n")
	gb.WriteString(fmt.Sprintf("    interval: %d\n", clusterHealthInterval))
	gb.WriteString("    proxies:\n")
	for _, n := range proxyNames {
		gb.WriteString("      - " + yamlQuote(n) + "\n")
	}
	return gb.String()
}

func clashSelectGroupYaml(groupName string, members []string) string {
	if len(members) == 0 {
		return ""
	}
	var gb strings.Builder
	gb.WriteString(fmt.Sprintf("  - name: %s\n    type: select\n    proxies:\n", yamlQuote(groupName)))
	for _, m := range members {
		gb.WriteString("      - " + m + "\n")
	}
	gb.WriteString("      - DIRECT\n")
	return gb.String()
}

func clashFallbackGroupYaml(groupName string, chain []string) string {
	if len(chain) == 0 {
		return ""
	}
	var gb strings.Builder
	gb.WriteString(fmt.Sprintf("  - name: %s\n", yamlQuote(groupName)))
	gb.WriteString("    type: fallback\n")
	gb.WriteString("    url: " + clusterHealthURL + "\n")
	gb.WriteString(fmt.Sprintf("    interval: %d\n", clusterHealthInterval))
	gb.WriteString("    proxies:\n")
	for _, n := range chain {
		gb.WriteString("      - " + yamlQuote(n) + "\n")
	}
	return gb.String()
}

func yamlQuote(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, ":#\n\r\t'\"{}[]&*!|>%@`, ") {
		return strconv.Quote(s)
	}
	return s
}

func yamlLine(key string, value interface{}, quoted bool) string {
	switch v := value.(type) {
	case bool:
		return fmt.Sprintf("%s: %t", key, v)
	case int:
		return fmt.Sprintf("%s: %d", key, v)
	default:
		val := fmt.Sprint(v)
		if quoted {
			val = yamlQuote(val)
		}
		return fmt.Sprintf("%s: %s", key, val)
	}
}

func inboundToClashLines(ib *model.Inbound, subHost string, requestHost string) ([]string, string, bool) {
	name := clashProxyName(ib)
	server := ResolveInboundAddress(ib, subHost, requestHost)
	st := parseStreamSettings(ib.StreamSettings)
	var lines []string
	add := func(k string, v interface{}, q bool) {
		lines = append(lines, yamlLine(k, v, q))
	}

	switch ib.Protocol {
	case model.VMess:
		var settings struct {
			Clients []struct {
				ID      string `json:"id"`
				AlterID int    `json:"alterId"`
			} `json:"clients"`
		}
		if json.Unmarshal([]byte(ib.Settings), &settings) != nil || len(settings.Clients) == 0 {
			return nil, "", false
		}
		c := settings.Clients[0]
		add("name", name, true)
		add("type", "vmess", true)
		add("server", server, true)
		add("port", ib.Port, false)
		add("uuid", c.ID, true)
		add("alterId", c.AlterID, false)
		add("cipher", "auto", true)
		add("udp", true, false)
		appendClashTLS(&lines, st)
		if st.Network != "" && st.Network != "tcp" {
			add("network", st.Network, true)
		}
		appendClashNetwork(&lines, st)
	case model.VLESS:
		var settings struct {
			Clients []struct {
				ID   string `json:"id"`
				Flow string `json:"flow"`
			} `json:"clients"`
		}
		if json.Unmarshal([]byte(ib.Settings), &settings) != nil || len(settings.Clients) == 0 {
			return nil, "", false
		}
		c := settings.Clients[0]
		add("name", name, true)
		add("type", "vless", true)
		add("server", server, true)
		add("port", ib.Port, false)
		add("uuid", c.ID, true)
		add("udp", true, false)
		if st.Security == "xtls" && c.Flow != "" {
			add("flow", c.Flow, true)
		}
		appendClashTLS(&lines, st)
		if st.Network != "" && st.Network != "tcp" {
			add("network", st.Network, true)
		}
		appendClashNetwork(&lines, st)
	case model.Trojan:
		var settings struct {
			Clients []struct {
				Password string `json:"password"`
			} `json:"clients"`
		}
		if json.Unmarshal([]byte(ib.Settings), &settings) != nil || len(settings.Clients) == 0 {
			return nil, "", false
		}
		add("name", name, true)
		add("type", "trojan", true)
		add("server", server, true)
		add("port", ib.Port, false)
		add("password", settings.Clients[0].Password, true)
		add("udp", true, false)
		appendClashTLS(&lines, st)
	case model.Shadowsocks:
		var settings struct {
			Method   string `json:"method"`
			Password string `json:"password"`
		}
		if json.Unmarshal([]byte(ib.Settings), &settings) != nil {
			return nil, "", false
		}
		add("name", name, true)
		add("type", "ss", true)
		add("server", server, true)
		add("port", ib.Port, false)
		add("cipher", settings.Method, true)
		add("password", settings.Password, true)
		add("udp", true, false)
	default:
		return nil, "", false
	}
	return lines, name, true
}

func clashProxyName(ib *model.Inbound) string {
	n := strings.TrimSpace(ib.Remark)
	if n == "" {
		n = fmt.Sprintf("inbound-%d", ib.Port)
	}
	n = strings.ReplaceAll(n, "\r", "")
	n = strings.ReplaceAll(n, "\n", " ")
	if len(n) > 64 {
		n = n[:64]
	}
	return n
}

func appendClashTLS(lines *[]string, st *streamSettings) {
	if st.Security == "tls" || st.Security == "xtls" {
		*lines = append(*lines, yamlLine("tls", true, false))
		*lines = append(*lines, yamlLine("skip-cert-verify", true, false))
		sni := ""
		if st.TLSSettings != nil {
			sni = st.TLSSettings.ServerName
		}
		if sni == "" && st.XTlsSettings != nil {
			sni = st.XTlsSettings.ServerName
		}
		if sni != "" {
			*lines = append(*lines, yamlLine("servername", sni, true))
		}
	}
}

func appendClashNetwork(lines *[]string, st *streamSettings) {
	switch st.Network {
	case "ws":
		if st.WSSettings == nil {
			return
		}
		*lines = append(*lines, "    ws-opts:")
		*lines = append(*lines, "      path: "+yamlQuote(st.WSSettings.Path))
		if h := headerValue(st.WSSettings.Headers, "host"); h != "" {
			*lines = append(*lines, "      headers:")
			*lines = append(*lines, "        Host: "+yamlQuote(h))
		}
	case "grpc":
		if st.GRPCSettings == nil {
			return
		}
		*lines = append(*lines, "    grpc-opts:")
		*lines = append(*lines, "      grpc-service-name: "+yamlQuote(st.GRPCSettings.ServiceName))
	}
}
