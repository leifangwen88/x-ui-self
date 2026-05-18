package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"x-ui/database/model"
)

const (
	XrayGroupSingleSite = ClashGroupSingleSite
	XrayGroupClusterLB  = ClashGroupClusterLB
)

type xrayBuildCtx struct {
	outbounds []map[string]interface{}
	balancers []map[string]interface{}
	usedTags  map[string]bool
}

type xrayClientConfig struct {
	Remarks   string                   `json:"remarks,omitempty"`
	Log       map[string]interface{}   `json:"log,omitempty"`
	Outbounds []map[string]interface{} `json:"outbounds"`
	Routing   map[string]interface{}   `json:"routing"`
}

func newXrayBuildCtx() *xrayBuildCtx {
	return &xrayBuildCtx{usedTags: make(map[string]bool)}
}

func (ctx *xrayBuildCtx) uniqueTag(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "node"
	}
	tag := base
	n := 2
	for ctx.usedTags[tag] {
		tag = fmt.Sprintf("%s-%d", base, n)
		n++
	}
	ctx.usedTags[tag] = true
	return tag
}

func (ctx *xrayBuildCtx) addOutbound(ob map[string]interface{}) string {
	name, _ := ob["tag"].(string)
	tag := ctx.uniqueTag(name)
	ob["tag"] = tag
	ctx.outbounds = append(ctx.outbounds, ob)
	return tag
}

func (ctx *xrayBuildCtx) addBalancer(tag string, selector []string, fallbackTag string) string {
	tag = ctx.uniqueTag(tag)
	ctx.balancers = append(ctx.balancers, xrayBalancerMap(tag, selector, fallbackTag))
	return tag
}

func xrayBalancerMap(tag string, selector []string, fallbackTag string) map[string]interface{} {
	b := map[string]interface{}{
		"tag":      tag,
		"selector": selector,
		"strategy": map[string]interface{}{"type": "roundRobin"},
		"healthCheck": map[string]interface{}{
			"enabled":  true,
			"interval": clusterHealthInterval,
			"url":      clusterHealthURL,
			"timeout":  clusterHealthTimeoutMs / 1000,
		},
	}
	if strings.TrimSpace(fallbackTag) != "" {
		b["fallbackTag"] = fallbackTag
	}
	return b
}

func marshalXrayClientConfig(cfg *xrayClientConfig) string {
	if cfg == nil {
		return ""
	}
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(cfg)
	return strings.TrimSpace(buf.String())
}

func xrayFreedomOutbound(tag string) map[string]interface{} {
	return map[string]interface{}{
		"tag":      tag,
		"protocol": "freedom",
		"settings": map[string]interface{}{},
	}
}

func xrayDefaultRouting(defaultBalancer string) map[string]interface{} {
	return map[string]interface{}{
		"domainStrategy": "AsIs",
		"rules": []map[string]interface{}{{
			"type":        "field",
			"network":     "tcp,udp",
			"balancerTag": defaultBalancer,
		}},
	}
}

func (ctx *xrayBuildCtx) finalize(defaultBalancer string) *xrayClientConfig {
	if len(ctx.outbounds) == 0 || strings.TrimSpace(defaultBalancer) == "" {
		return nil
	}
	ctx.addOutbound(xrayFreedomOutbound("DIRECT"))
	routing := xrayDefaultRouting(defaultBalancer)
	if len(ctx.balancers) > 0 {
		routing["balancers"] = ctx.balancers
	}
	return &xrayClientConfig{
		Outbounds: ctx.outbounds,
		Routing:   routing,
	}
}

func buildXrayStreamSettings(st *streamSettings) map[string]interface{} {
	if st == nil {
		return map[string]interface{}{"network": "tcp"}
	}
	m := map[string]interface{}{
		"network": st.Network,
	}
	if st.Network == "" {
		m["network"] = "tcp"
	}
	if st.Security == "tls" || st.Security == "xtls" {
		m["security"] = st.Security
		tls := map[string]interface{}{"allowInsecure": true}
		sni := ""
		if st.TLSSettings != nil {
			sni = st.TLSSettings.ServerName
		}
		if sni == "" && st.XTlsSettings != nil {
			sni = st.XTlsSettings.ServerName
		}
		if sni != "" {
			tls["serverName"] = sni
		}
		if st.Security == "xtls" {
			m["xtlsSettings"] = tls
		} else {
			m["tlsSettings"] = tls
		}
	}
	switch st.Network {
	case "ws":
		if st.WSSettings != nil {
			ws := map[string]interface{}{"path": st.WSSettings.Path}
			if h := headerValue(st.WSSettings.Headers, "host"); h != "" {
				ws["headers"] = map[string]interface{}{"Host": h}
			}
			m["wsSettings"] = ws
		}
	case "grpc":
		if st.GRPCSettings != nil {
			m["grpcSettings"] = map[string]interface{}{
				"serviceName": st.GRPCSettings.ServiceName,
			}
		}
	case "tcp":
		typ, host, path := vmessTCPParams(st.TCPSettings)
		if typ == "http" && (host != "" || path != "") {
			tcp := map[string]interface{}{"header": map[string]interface{}{"type": "http"}}
			req := map[string]interface{}{}
			if path != "" {
				req["path"] = []string{path}
			}
			if host != "" {
				req["headers"] = map[string]interface{}{"Host": []string{host}}
			}
			if len(req) > 0 {
				tcp["request"] = req
			}
			m["tcpSettings"] = tcp
		}
	case "http", "h2":
		if st.HTTPSettings != nil {
			h2 := map[string]interface{}{}
			if len(st.HTTPSettings.Host) > 0 {
				h2["host"] = st.HTTPSettings.Host
			}
			if len(st.HTTPSettings.Path) > 0 {
				h2["path"] = st.HTTPSettings.Path[0]
			}
			m["httpSettings"] = h2
		}
	}
	return m
}

func inboundToXrayOutbound(ib *model.Inbound, tag, subHost, requestHost string) (map[string]interface{}, bool) {
	if strings.TrimSpace(tag) == "" {
		tag = clashProxyName(ib)
	}
	server := ResolveInboundAddress(ib, subHost, requestHost)
	st := parseStreamSettings(ib.StreamSettings)
	stream := buildXrayStreamSettings(st)

	switch ib.Protocol {
	case model.VMess:
		var settings struct {
			Clients []struct {
				ID      string `json:"id"`
				AlterID int    `json:"alterId"`
			} `json:"clients"`
		}
		if json.Unmarshal([]byte(ib.Settings), &settings) != nil || len(settings.Clients) == 0 {
			return nil, false
		}
		c := settings.Clients[0]
		return map[string]interface{}{
			"tag":      tag,
			"protocol": "vmess",
			"settings": map[string]interface{}{
				"vnext": []map[string]interface{}{{
					"address": server,
					"port":    ib.Port,
					"users": []map[string]interface{}{{
						"id":       c.ID,
						"alterId":  c.AlterID,
						"security": "auto",
					}},
				}},
			},
			"streamSettings": stream,
		}, true
	case model.VLESS:
		var settings struct {
			Clients []struct {
				ID   string `json:"id"`
				Flow string `json:"flow"`
			} `json:"clients"`
		}
		if json.Unmarshal([]byte(ib.Settings), &settings) != nil || len(settings.Clients) == 0 {
			return nil, false
		}
		c := settings.Clients[0]
		user := map[string]interface{}{
			"id":         c.ID,
			"encryption": "none",
		}
		if st.Security == "xtls" && c.Flow != "" {
			user["flow"] = c.Flow
		}
		return map[string]interface{}{
			"tag":      tag,
			"protocol": "vless",
			"settings": map[string]interface{}{
				"vnext": []map[string]interface{}{{
					"address": server,
					"port":    ib.Port,
					"users":   []map[string]interface{}{user},
				}},
			},
			"streamSettings": stream,
		}, true
	case model.Trojan:
		var settings struct {
			Clients []struct {
				Password string `json:"password"`
			} `json:"clients"`
		}
		if json.Unmarshal([]byte(ib.Settings), &settings) != nil || len(settings.Clients) == 0 {
			return nil, false
		}
		return map[string]interface{}{
			"tag":      tag,
			"protocol": "trojan",
			"settings": map[string]interface{}{
				"servers": []map[string]interface{}{{
					"address":  server,
					"port":     ib.Port,
					"password": settings.Clients[0].Password,
				}},
			},
			"streamSettings": stream,
		}, true
	case model.Shadowsocks:
		var settings struct {
			Method   string `json:"method"`
			Password string `json:"password"`
		}
		if json.Unmarshal([]byte(ib.Settings), &settings) != nil {
			return nil, false
		}
		return map[string]interface{}{
			"tag":      tag,
			"protocol": "shadowsocks",
			"settings": map[string]interface{}{
				"servers": []map[string]interface{}{{
					"address":  server,
					"port":     ib.Port,
					"method":   settings.Method,
					"password": settings.Password,
				}},
			},
			"streamSettings": stream,
		}, true
	default:
		return nil, false
	}
}

func addInboundXrayOutbound(ctx *xrayBuildCtx, ib *model.Inbound, proxyName, subHost, requestHost string) (string, bool) {
	if strings.TrimSpace(proxyName) == "" {
		proxyName = clashProxyName(ib)
	}
	ob, ok := inboundToXrayOutbound(ib, proxyName, subHost, requestHost)
	if !ok {
		return "", false
	}
	return ctx.addOutbound(ob), true
}

func buildClusterInboundGroupXray(ctx *xrayBuildCtx, groupName string, primary, fallback []string) string {
	gn := strings.TrimSpace(groupName)
	if gn == "" {
		return ""
	}
	if len(primary) == 0 && len(fallback) == 0 {
		return ""
	}
	primaryPick := gn + "-首选"
	if len(primary) == 0 {
		return buildXrayFallbackChain(ctx, gn, fallback)
	}
	if len(fallback) == 0 {
		if len(primary) == 1 {
			return primary[0]
		}
		return ctx.addBalancer(primaryPick, primary, "")
	}
	chain := make([]string, 0, 1+len(fallback))
	if len(primary) == 1 {
		chain = append(chain, primary[0])
	} else {
		chain = append(chain, ctx.addBalancer(primaryPick, primary, ""))
	}
	chain = append(chain, fallback...)
	return buildXrayFallbackChain(ctx, gn, chain)
}

func buildXrayFallbackChain(ctx *xrayBuildCtx, groupName string, chain []string) string {
	if len(chain) == 0 {
		return ""
	}
	if len(chain) == 1 {
		return chain[0]
	}
	downstream := chain[len(chain)-1]
	for i := len(chain) - 2; i >= 0; i-- {
		tag := groupName
		if i < len(chain)-2 {
			tag = fmt.Sprintf("%s-fb%d", groupName, i)
		}
		downstream = ctx.addBalancer(tag, []string{chain[i]}, downstream)
	}
	return downstream
}

func GenXrayJsonByGame(inbounds []*model.Inbound, subHost, requestHost string) string {
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

	ctx := newXrayBuildCtx()
	gameRoots := make([]string, 0)

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
		nodeTags := make([]string, 0)
		for _, ib := range list {
			tag, ok := addInboundXrayOutbound(ctx, ib, clashProxyName(ib), subHost, requestHost)
			if !ok {
				continue
			}
			nodeTags = append(nodeTags, tag)
		}
		if len(nodeTags) == 0 {
			continue
		}
		var gameRoot string
		if len(nodeTags) == 1 {
			gameRoot = nodeTags[0]
		} else {
			gameRoot = ctx.addBalancer(gn, nodeTags, "")
		}
		gameRoots = append(gameRoots, gameRoot)
	}
	if len(gameRoots) == 0 {
		return ""
	}
	defaultTag := gameRoots[0]
	if len(gameRoots) > 1 {
		defaultTag = ctx.addBalancer(XrayGroupSingleSite, gameRoots, "")
	}
	cfg := ctx.finalize(defaultTag)
	if cfg == nil {
		return ""
	}
	cfg.Remarks = "x-ui 单站 Xray JSON（含游戏分组与负载均衡）。V2Box：复制订阅 URL 或 JSON 导入；切换出站可选「" + XrayGroupSingleSite + "」或各游戏名。"
	return marshalXrayClientConfig(cfg)
}

func xrayTagsFromProxyNames(names []string, nameToTag map[string]string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if t, ok := nameToTag[n]; ok && t != "" {
			out = append(out, t)
		}
	}
	return out
}

func genClusterXrayJsonByGame(members map[string][]clusterMember, order []string, games []*model.Game) string {
	_, groups := buildClusterInboundGroups(members, order, games)
	if len(groups) == 0 {
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

	ctx := newXrayBuildCtx()
	nameToTag := make(map[string]string)
	for _, key := range order {
		list := members[key]
		if len(list) == 0 {
			continue
		}
		for _, m := range list {
			pname := displayInboundName(m.Inbound.Remark, m.Inbound.Port)
			if len(list) > 1 {
				pname = fmt.Sprintf("%s @ %s", pname, clusterSubDisplayLabel(m.NodeLabel, m.ConnectHost))
			}
			tag, ok := addInboundXrayOutbound(ctx, m.Inbound, pname, "", m.ConnectHost)
			if ok {
				nameToTag[pname] = tag
			}
		}
	}

	gameRoots := make([]string, 0)
	for _, gid := range gameOrder {
		inbounds := byGame[gid]
		if len(inbounds) == 0 {
			continue
		}
		gn := clusterGameName(gid, gameName)
		inboundRoots := make([]string, 0)
		for _, ig := range inbounds {
			primary := xrayTagsFromProxyNames(ig.Primary, nameToTag)
			fallback := xrayTagsFromProxyNames(ig.Fallback, nameToTag)
			root := buildClusterInboundGroupXray(ctx, ig.GroupName, primary, fallback)
			if root != "" {
				inboundRoots = append(inboundRoots, root)
			}
		}
		if len(inboundRoots) == 0 {
			continue
		}
		var gameRoot string
		if len(inboundRoots) == 1 {
			gameRoot = inboundRoots[0]
		} else {
			gameRoot = ctx.addBalancer(gn, inboundRoots, "")
		}
		gameRoots = append(gameRoots, gameRoot)
	}
	if len(gameRoots) == 0 {
		return ""
	}
	defaultTag := gameRoots[0]
	if len(gameRoots) > 1 {
		defaultTag = ctx.addBalancer(XrayGroupClusterLB, gameRoots, "")
	}
	cfg := ctx.finalize(defaultTag)
	if cfg == nil {
		return ""
	}
	cfg.Remarks = "x-ui 站群 Xray JSON（游戏→入站备注→首选负载均衡/兜底）。V2Box 导入后默认走「" + XrayGroupClusterLB + "」，可在出站列表切换游戏或入口组。"
	return marshalXrayClientConfig(cfg)
}
