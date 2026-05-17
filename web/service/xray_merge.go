package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"x-ui/database/model"
)

type routingRule map[string]interface{}

type routingConfig struct {
	Rules []routingRule `json:"rules"`
}

type inboundSocksBinding struct {
	inboundTag string
	socks      *model.SocksProxy
}

func socksOutboundTag(socksId int) string {
	return fmt.Sprintf("socks-%d", socksId)
}

func mergeSocksProxiesIntoConfig(templateOutboundJSON []byte, templateRoutingJSON []byte, bindings []inboundSocksBinding) ([]byte, []byte, error) {
	outbounds, err := parseOutboundList(templateOutboundJSON)
	if err != nil {
		return nil, nil, err
	}
	outbounds = filterSocksOutbounds(outbounds)

	usedSocks := make(map[int]bool)
	for _, b := range bindings {
		if usedSocks[b.socks.Id] {
			continue
		}
		usedSocks[b.socks.Id] = true
		outbounds = append(outbounds, buildSocksOutbound(b.socks))
	}

	routing, err := parseRoutingConfig(templateRoutingJSON)
	if err != nil {
		return nil, nil, err
	}
	routing.Rules = filterSocksRoutingRules(routing.Rules)
	for _, b := range bindings {
		routing.Rules = append(routing.Rules, routingRule{
			"type":        "field",
			"inboundTag":  []string{b.inboundTag},
			"outboundTag": socksOutboundTag(b.socks.Id),
		})
	}

	outboundJSON, err := json.Marshal(outbounds)
	if err != nil {
		return nil, nil, err
	}
	routingJSON, err := json.Marshal(routing)
	if err != nil {
		return nil, nil, err
	}
	return outboundJSON, routingJSON, nil
}

func parseOutboundList(raw []byte) ([]map[string]interface{}, error) {
	if len(raw) == 0 {
		return []map[string]interface{}{}, nil
	}
	var outbounds []map[string]interface{}
	if err := json.Unmarshal(raw, &outbounds); err != nil {
		return nil, err
	}
	return outbounds, nil
}

func parseRoutingConfig(raw []byte) (*routingConfig, error) {
	if len(raw) == 0 {
		return &routingConfig{Rules: []routingRule{}}, nil
	}
	routing := &routingConfig{}
	if err := json.Unmarshal(raw, routing); err != nil {
		return nil, err
	}
	return routing, nil
}

func filterSocksOutbounds(outbounds []map[string]interface{}) []map[string]interface{} {
	filtered := make([]map[string]interface{}, 0, len(outbounds))
	for _, ob := range outbounds {
		tag, _ := ob["tag"].(string)
		if strings.HasPrefix(tag, "socks-") {
			continue
		}
		filtered = append(filtered, ob)
	}
	return filtered
}

func filterSocksRoutingRules(rules []routingRule) []routingRule {
	filtered := make([]routingRule, 0, len(rules))
	for _, rule := range rules {
		outTag, _ := rule["outboundTag"].(string)
		if strings.HasPrefix(outTag, "socks-") {
			continue
		}
		filtered = append(filtered, rule)
	}
	return filtered
}

func buildSocksOutbound(socks *model.SocksProxy) map[string]interface{} {
	return map[string]interface{}{
		"tag":      socksOutboundTag(socks.Id),
		"protocol": "socks",
		"settings": map[string]interface{}{
			"servers": []map[string]interface{}{
				{
					"address": socks.Address,
					"port":    socks.Port,
					"users": []map[string]interface{}{
						{
							"user": socks.Username,
							"pass": socks.Password,
						},
					},
				},
			},
		},
	}
}

func parseSocksProxyLine(line string) (*model.SocksProxy, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}
	parts := strings.Split(line, ":")
	if len(parts) != 4 {
		return nil, fmt.Errorf("格式错误，应为 服务器:端口:账号:密码 -> %s", line)
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("端口无效: %s", parts[1])
	}
	return &model.SocksProxy{
		Address:  parts[0],
		Port:     port,
		Username: parts[2],
		Password: parts[3],
		Enable:   true,
	}, nil
}
