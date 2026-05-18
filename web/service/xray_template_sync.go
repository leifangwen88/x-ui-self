package service

import (
	"encoding/json"
	"strings"
	"time"
	"x-ui/database"
	"x-ui/database/model"

	"gorm.io/gorm"
)

type TemplateSyncResult struct {
	SocksCreated   int `json:"socksCreated"`
	SocksMatched   int `json:"socksMatched"`
	InboundBound   int `json:"inboundBound"`
	InboundSkipped int `json:"inboundSkipped"`
}

type socksOutboundParsed struct {
	Tag      string
	Address  string
	Port     int
	Username string
	Password string
}

func (s *SocksProxyService) SyncBindingsFromTemplate(templateJSON string) (*TemplateSyncResult, error) {
	outboundMap, rules, err := parseTemplateSocksBindings(templateJSON)
	if err != nil {
		return nil, err
	}
	result := &TemplateSyncResult{}
	db := database.GetDB()

	var inbounds []*model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		return nil, err
	}
	inboundByTag := make(map[string]*model.Inbound, len(inbounds))
	for _, inbound := range inbounds {
		inboundByTag[inbound.Tag] = inbound
	}

	for _, rule := range rules {
		inboundTag := firstString(rule["inboundTag"])
		outboundTag, _ := rule["outboundTag"].(string)
		if inboundTag == "" || !strings.HasPrefix(outboundTag, "socks-") {
			continue
		}
		socksOb, ok := outboundMap[outboundTag]
		if !ok {
			continue
		}
		inbound := inboundByTag[inboundTag]
		if inbound == nil {
			result.InboundSkipped++
			continue
		}

		socksId, created, err := s.upsertSocksProxy(socksOb)
		if err != nil {
			return nil, err
		}
		if created {
			result.SocksCreated++
		} else {
			result.SocksMatched++
		}
		if sp, err := s.GetById(socksId); err == nil {
			EmitSocksUpsert(sp)
		}
		if inbound.SocksProxyId != socksId {
			inbound.SocksProxyId = socksId
			if err := db.Model(inbound).Update("socks_proxy_id", socksId).Error; err != nil {
				return nil, err
			}
			result.InboundBound++
			EmitInboundUpsert(inbound)
		}
	}
	return result, nil
}

func parseTemplateSocksBindings(templateJSON string) (map[string]socksOutboundParsed, []routingRule, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(templateJSON), &root); err != nil {
		return nil, nil, err
	}

	outboundMap := make(map[string]socksOutboundParsed)
	if raw, ok := root["outbounds"]; ok {
		var outbounds []map[string]interface{}
		if err := json.Unmarshal(raw, &outbounds); err != nil {
			return nil, nil, err
		}
		for _, ob := range outbounds {
			tag, _ := ob["tag"].(string)
			if !strings.HasPrefix(tag, "socks-") {
				continue
			}
			parsed, ok := parseSocksOutboundMap(ob)
			if !ok {
				continue
			}
			parsed.Tag = tag
			outboundMap[tag] = parsed
		}
	}

	var rules []routingRule
	if raw, ok := root["routing"]; ok {
		routing, err := parseRoutingConfig(raw)
		if err != nil {
			return nil, nil, err
		}
		rules = routing.Rules
	}
	return outboundMap, rules, nil
}

func parseSocksOutboundMap(ob map[string]interface{}) (socksOutboundParsed, bool) {
	protocol, _ := ob["protocol"].(string)
	if protocol != "socks" {
		return socksOutboundParsed{}, false
	}
	settings, _ := ob["settings"].(map[string]interface{})
	servers, _ := settings["servers"].([]interface{})
	if len(servers) == 0 {
		return socksOutboundParsed{}, false
	}
	server, _ := servers[0].(map[string]interface{})
	address, _ := server["address"].(string)
	portF, _ := server["port"].(float64)
	port := int(portF)
	user, pass := "", ""
	if users, ok := server["users"].([]interface{}); ok && len(users) > 0 {
		if u, ok := users[0].(map[string]interface{}); ok {
			user, _ = u["user"].(string)
			pass, _ = u["pass"].(string)
		}
	}
	if address == "" || port <= 0 {
		return socksOutboundParsed{}, false
	}
	return socksOutboundParsed{
		Address:  address,
		Port:     port,
		Username: user,
		Password: pass,
	}, true
}

func firstString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []interface{}:
		if len(t) > 0 {
			if s, ok := t[0].(string); ok {
				return s
			}
		}
	case []string:
		if len(t) > 0 {
			return t[0]
		}
	}
	return ""
}

func (s *SocksProxyService) upsertSocksProxy(ob socksOutboundParsed) (int, bool, error) {
	db := database.GetDB()
	var exist model.SocksProxy
	err := db.Where("address = ? AND port = ?", ob.Address, ob.Port).First(&exist).Error
	if err == nil {
		updates := map[string]interface{}{
			"username": ob.Username,
			"password": ob.Password,
			"enable":   true,
		}
		if err := db.Model(&exist).Updates(updates).Error; err != nil {
			return 0, false, err
		}
		if row, err := s.GetById(exist.Id); err == nil {
			EmitSocksUpsert(row)
		}
		return exist.Id, false, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return 0, false, err
	}
	now := time.Now().UnixMilli()
	socks := &model.SocksProxy{
		Address:    ob.Address,
		Port:       ob.Port,
		Username:   ob.Username,
		Password:   ob.Password,
		Enable:     true,
		CreatedAt:  now,
		ExpiryTime: 0,
	}
	if err := db.Create(socks).Error; err != nil {
		return 0, false, err
	}
	EmitSocksUpsert(socks)
	return socks.Id, true, nil
}
