package service

import (
	"encoding/base64"
	"strings"

	"x-ui/database/model"
)

type SubscriptionService struct {
	settingService SettingService
	inboundService InboundService
}

type SubInfo struct {
	Token      string `json:"token"`
	SubHost    string `json:"subHost"`
	Base64Path string `json:"base64Path"`
	LinksPath  string `json:"linksPath"`
	ClashPath  string `json:"clashPath"`
}

func (s *SubscriptionService) GetInfo() (*SubInfo, error) {
	token, err := s.settingService.GetSubToken()
	if err != nil {
		return nil, err
	}
	subHost, err := s.settingService.GetSubHost()
	if err != nil {
		return nil, err
	}
	return &SubInfo{
		Token:      token,
		SubHost:    subHost,
		Base64Path: "sub/" + token,
		LinksPath:  "sub/" + token + "?type=links",
		ClashPath:  "sub/" + token + "?type=clash",
	}, nil
}

func (s *SubscriptionService) ResetToken() (string, error) {
	return s.settingService.ResetSubToken()
}

func (s *SubscriptionService) ValidateToken(token string) bool {
	expected, err := s.settingService.GetSubToken()
	return err == nil && expected != "" && token == expected
}

func (s *SubscriptionService) CollectShareLinks(subHost string, requestHost string) []string {
	inbounds, err := s.inboundService.GetAllInbounds()
	if err != nil {
		return nil
	}
	links := make([]string, 0, len(inbounds))
	for _, ib := range inbounds {
		link := GenInboundShareLink(ib, subHost, requestHost)
		if link != "" {
			links = append(links, link)
		}
	}
	return links
}

func (s *SubscriptionService) GenBase64Subscription(subHost string, requestHost string) string {
	links := s.CollectShareLinks(subHost, requestHost)
	if len(links) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n")))
}

func (s *SubscriptionService) GenLinksText(subHost string, requestHost string) string {
	links := s.CollectShareLinks(subHost, requestHost)
	return strings.Join(links, "\n")
}

func (s *SubscriptionService) GenClashSubscription(subHost string, requestHost string) string {
	inbounds, err := s.inboundService.GetAllInbounds()
	if err != nil {
		return ""
	}
	return GenClashYaml(inbounds, subHost, requestHost)
}
