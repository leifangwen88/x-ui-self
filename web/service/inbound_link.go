package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

	"x-ui/database/model"
)

type streamSettings struct {
	Network      string          `json:"network"`
	Security     string          `json:"security"`
	TLSSettings  *tlsSettings    `json:"tlsSettings"`
	XTlsSettings *tlsSettings    `json:"xtlsSettings"`
	TCPSettings  json.RawMessage `json:"tcpSettings"`
	WSSettings   *wsSettings     `json:"wsSettings"`
	HTTPSettings *httpSettings   `json:"httpSettings"`
	GRPCSettings *grpcSettings   `json:"grpcSettings"`
	KCPSettings  *kcpSettings    `json:"kcpSettings"`
	QUICSettings *quicSettings   `json:"quicSettings"`
}

type tlsSettings struct {
	ServerName string `json:"serverName"`
}

type wsSettings struct {
	Path    string           `json:"path"`
	Headers *json.RawMessage `json:"headers"`
}

type httpSettings struct {
	Path []string `json:"path"`
	Host []string `json:"host"`
}

type grpcSettings struct {
	ServiceName string `json:"serviceName"`
}

type kcpSettings struct {
	Type string `json:"type"`
	Seed string `json:"seed"`
}

type quicSettings struct {
	Security string `json:"security"`
	Key      string `json:"key"`
	Type     string `json:"type"`
}

func parseStreamSettings(raw string) *streamSettings {
	if raw == "" || raw == "{}" {
		return &streamSettings{Network: "tcp", Security: "none"}
	}
	st := &streamSettings{}
	if err := json.Unmarshal([]byte(raw), st); err != nil {
		return &streamSettings{Network: "tcp", Security: "none"}
	}
	if st.Network == "" {
		st.Network = "tcp"
	}
	return st
}

func hostOnly(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func ResolveInboundAddress(inbound *model.Inbound, subHost string, requestHost string) string {
	address := hostOnly(requestHost)
	if inbound.Listen != "" && inbound.Listen != "0.0.0.0" {
		address = inbound.Listen
	}
	if subHost != "" {
		address = strings.TrimSpace(subHost)
	}
	st := parseStreamSettings(inbound.StreamSettings)
	if st.Security == "tls" && st.TLSSettings != nil && st.TLSSettings.ServerName != "" {
		address = st.TLSSettings.ServerName
	}
	if st.Security == "xtls" && st.XTlsSettings != nil && st.XTlsSettings.ServerName != "" {
		address = st.XTlsSettings.ServerName
	}
	return address
}

func inboundRemark(inbound *model.Inbound) string {
	if strings.TrimSpace(inbound.Remark) != "" {
		return inbound.Remark
	}
	return fmt.Sprintf("inbound-%d", inbound.Port)
}

func InboundSupportsLink(protocol model.Protocol) bool {
	switch protocol {
	case model.VMess, model.VLESS, model.Trojan, model.Shadowsocks:
		return true
	default:
		return false
	}
}

func GenInboundShareLink(inbound *model.Inbound, subHost string, requestHost string) string {
	if inbound == nil || !inbound.Enable || !InboundSupportsLink(inbound.Protocol) {
		return ""
	}
	address := ResolveInboundAddress(inbound, subHost, requestHost)
	remark := inboundRemark(inbound)
	switch inbound.Protocol {
	case model.VMess:
		return genVmessLink(inbound, address, remark)
	case model.VLESS:
		return genVLESSLink(inbound, address, remark)
	case model.Trojan:
		return genTrojanLink(inbound, address, remark)
	case model.Shadowsocks:
		return genShadowsocksLink(inbound, address, remark)
	default:
		return ""
	}
}

func genVmessLink(inbound *model.Inbound, address string, remark string) string {
	var settings struct {
		Clients []struct {
			ID      string `json:"id"`
			AlterID int    `json:"alterId"`
		} `json:"clients"`
	}
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil || len(settings.Clients) == 0 {
		return ""
	}
	client := settings.Clients[0]
	st := parseStreamSettings(inbound.StreamSettings)

	network := st.Network
	typ := "none"
	host := ""
	path := ""

	switch network {
	case "tcp":
		typ, host, path = vmessTCPParams(st.TCPSettings)
	case "kcp":
		if st.KCPSettings != nil {
			typ = st.KCPSettings.Type
			path = st.KCPSettings.Seed
		}
	case "ws":
		if st.WSSettings != nil {
			path = st.WSSettings.Path
			host = headerValue(st.WSSettings.Headers, "host")
		}
	case "http":
		network = "h2"
		if st.HTTPSettings != nil {
			if len(st.HTTPSettings.Path) > 0 {
				path = st.HTTPSettings.Path[0]
			}
			if len(st.HTTPSettings.Host) > 0 {
				host = strings.Join(st.HTTPSettings.Host, ",")
			}
		}
	case "quic":
		if st.QUICSettings != nil {
			typ = st.QUICSettings.Type
			host = st.QUICSettings.Security
			path = st.QUICSettings.Key
		}
	case "grpc":
		if st.GRPCSettings != nil {
			path = st.GRPCSettings.ServiceName
		}
	}

	obj := map[string]interface{}{
		"v":    "2",
		"ps":   remark,
		"add":  address,
		"port": inbound.Port,
		"id":   client.ID,
		"aid":  client.AlterID,
		"net":  network,
		"type": typ,
		"host": host,
		"path": path,
		"tls":  st.Security,
	}
	raw, _ := json.Marshal(obj)
	return "vmess://" + base64.StdEncoding.EncodeToString(raw)
}

func vmessTCPParams(raw json.RawMessage) (typ string, host string, path string) {
	typ = "none"
	if len(raw) == 0 {
		return
	}
	var tcp struct {
		Type    string `json:"type"`
		Request struct {
			Path    []string `json:"path"`
			Headers struct {
				Host []string `json:"Host"`
			} `json:"headers"`
		} `json:"request"`
	}
	if json.Unmarshal(raw, &tcp) != nil {
		return
	}
	typ = tcp.Type
	if tcp.Type == "http" {
		path = strings.Join(tcp.Request.Path, ",")
		if len(tcp.Request.Headers.Host) > 0 {
			host = tcp.Request.Headers.Host[0]
		}
	}
	return
}

func headerValue(headers *json.RawMessage, name string) string {
	if headers == nil || len(*headers) == 0 {
		return ""
	}
	var m map[string]interface{}
	if json.Unmarshal(*headers, &m) != nil {
		return ""
	}
	for k, v := range m {
		if strings.EqualFold(k, name) {
			switch t := v.(type) {
			case string:
				return t
			case []interface{}:
				if len(t) > 0 {
					if s, ok := t[0].(string); ok {
						return s
					}
				}
			}
		}
	}
	return ""
}

func genVLESSLink(inbound *model.Inbound, address string, remark string) string {
	var settings struct {
		Clients []struct {
			ID   string `json:"id"`
			Flow string `json:"flow"`
		} `json:"clients"`
	}
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil || len(settings.Clients) == 0 {
		return ""
	}
	client := settings.Clients[0]
	st := parseStreamSettings(inbound.StreamSettings)

	params := url.Values{}
	params.Set("type", st.Network)
	if st.Security == "xtls" {
		params.Set("security", "xtls")
	} else {
		params.Set("security", st.Security)
	}

	switch st.Network {
	case "ws":
		if st.WSSettings != nil {
			params.Set("path", st.WSSettings.Path)
			if h := headerValue(st.WSSettings.Headers, "host"); h != "" {
				params.Set("host", h)
			}
		}
	case "grpc":
		if st.GRPCSettings != nil {
			params.Set("serviceName", st.GRPCSettings.ServiceName)
		}
	case "http":
		if st.HTTPSettings != nil && len(st.HTTPSettings.Path) > 0 {
			params.Set("path", st.HTTPSettings.Path[0])
		}
		if st.HTTPSettings != nil && len(st.HTTPSettings.Host) > 0 {
			params.Set("host", strings.Join(st.HTTPSettings.Host, ","))
		}
	}

	if st.Security == "tls" || st.Security == "xtls" {
		if st.TLSSettings != nil && st.TLSSettings.ServerName != "" {
			params.Set("sni", st.TLSSettings.ServerName)
		} else if st.XTlsSettings != nil && st.XTlsSettings.ServerName != "" {
			params.Set("sni", st.XTlsSettings.ServerName)
		}
	}
	if st.Security == "xtls" && client.Flow != "" {
		params.Set("flow", client.Flow)
	}

	link := fmt.Sprintf("vless://%s@%s:%d?%s", client.ID, address, inbound.Port, params.Encode())
	return link + "#" + url.QueryEscape(remark)
}

func genTrojanLink(inbound *model.Inbound, address string, remark string) string {
	var settings struct {
		Clients []struct {
			Password string `json:"password"`
		} `json:"clients"`
	}
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil || len(settings.Clients) == 0 {
		return ""
	}
	password := settings.Clients[0].Password
	return fmt.Sprintf("trojan://%s@%s:%d#%s", password, address, inbound.Port, url.QueryEscape(remark))
}

func genShadowsocksLink(inbound *model.Inbound, address string, remark string) string {
	var settings struct {
		Method   string `json:"method"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		return ""
	}
	userinfo := fmt.Sprintf("%s:%s@%s:%d", settings.Method, settings.Password, address, inbound.Port)
	encoded := base64.StdEncoding.EncodeToString([]byte(userinfo))
	encoded = strings.NewReplacer("+", "-", "/", "_", "=", "").Replace(encoded)
	return fmt.Sprintf("ss://%s#%s", encoded, url.QueryEscape(remark))
}
