package service

import (
	"encoding/base64"
	"strings"
)

// EncodeV2raySubscription 生成 v2rayNG / v2rayN 等客户端可用的 Base64 订阅正文（仅含分享链接行）
func EncodeV2raySubscription(links []string) string {
	filtered := filterV2rayShareLinks(links)
	if len(filtered) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(filtered, "\n")))
}

func filterV2rayShareLinks(links []string) []string {
	out := make([]string, 0, len(links))
	for _, line := range links {
		line = strings.TrimSpace(line)
		if isV2rayShareLink(line) {
			out = append(out, line)
		}
	}
	return out
}

func shareLinksFromSubscriptionText(text string) []string {
	var links []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if isV2rayShareLink(line) {
			links = append(links, line)
		}
	}
	return links
}

func isV2rayShareLink(line string) bool {
	lower := strings.ToLower(line)
	return strings.HasPrefix(lower, "vmess://") ||
		strings.HasPrefix(lower, "vless://") ||
		strings.HasPrefix(lower, "trojan://") ||
		strings.HasPrefix(lower, "ss://")
}

func (s *SubscriptionService) GenV2raySubscription(subHost string, requestHost string, gameId int) string {
	return EncodeV2raySubscription(s.CollectShareLinks(subHost, requestHost, gameId))
}

func (s *SubscriptionService) GenClusterV2raySubscription(subHost string, requestHost string, gameId int) string {
	text := s.GenClusterLinksText(subHost, requestHost, gameId)
	return EncodeV2raySubscription(shareLinksFromSubscriptionText(text))
}
