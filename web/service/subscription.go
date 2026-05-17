package service

import (
	"encoding/base64"
	"fmt"
	"strings"

	"x-ui/database/model"
)

type SubscriptionService struct {
	settingService SettingService
	inboundService InboundService
	gameService    GameService
}

type SubGameGroup struct {
	GameId        int    `json:"gameId"`
	GameName      string `json:"gameName"`
	InboundCount  int    `json:"inboundCount"`
	Base64Path    string `json:"base64Path"`
	LinksPath     string `json:"linksPath"`
	ClashPath     string `json:"clashPath"`
}

type SubInfo struct {
	Token               string         `json:"token"`
	SubHost             string         `json:"subHost"`
	Base64Path          string         `json:"base64Path"`
	LinksPath           string         `json:"linksPath"`
	ClashPath           string         `json:"clashPath"`
	TotalInboundCount   int            `json:"totalInboundCount"`
	UnspecifiedCount    int            `json:"unspecifiedCount"`
	Groups              []SubGameGroup `json:"groups"`
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
	groups, total, unspecified, err := s.buildSubGroups(token)
	if err != nil {
		return nil, err
	}
	return &SubInfo{
		Token:             token,
		SubHost:           subHost,
		Base64Path:        "sub/" + token,
		LinksPath:         "sub/" + token + "?type=links",
		ClashPath:         "sub/" + token + "?type=clash",
		TotalInboundCount: total,
		UnspecifiedCount:  unspecified,
		Groups:            groups,
	}, nil
}

func (s *SubscriptionService) buildSubGroups(token string) ([]SubGameGroup, int, int, error) {
	games, err := s.gameService.GetAll()
	if err != nil {
		return nil, 0, 0, err
	}
	inbounds, err := s.inboundService.GetAllInbounds()
	if err != nil {
		return nil, 0, 0, err
	}
	countByGame := make(map[int]int)
	total := 0
	for _, ib := range inbounds {
		if !ib.Enable || !InboundSupportsLink(ib.Protocol) {
			continue
		}
		total++
		gid := ib.GameId
		if gid <= 0 {
			gid = 0
		}
		countByGame[gid]++
	}
	unspecified := countByGame[0]
	var groups []SubGameGroup
	for _, g := range games {
		n := countByGame[g.Id]
		if n == 0 {
			continue
		}
		prefix := fmt.Sprintf("sub/%s?gameId=%d", token, g.Id)
		groups = append(groups, SubGameGroup{
			GameId:       g.Id,
			GameName:     g.Name,
			InboundCount: n,
			Base64Path:   prefix,
			LinksPath:    prefix + "&type=links",
			ClashPath:    prefix + "&type=clash",
		})
	}
	return groups, total, unspecified, nil
}

func (s *SubscriptionService) ResetToken() (string, error) {
	return s.settingService.ResetSubToken()
}

func (s *SubscriptionService) ValidateToken(token string) bool {
	expected, err := s.settingService.GetSubToken()
	return err == nil && expected != "" && token == expected
}

func (s *SubscriptionService) filterInbounds(gameId int) ([]*model.Inbound, error) {
	inbounds, err := s.inboundService.GetAllInbounds()
	if err != nil {
		return nil, err
	}
	if gameId < 0 {
		list := make([]*model.Inbound, 0, len(inbounds))
		for _, ib := range inbounds {
			if ib.Enable && InboundSupportsLink(ib.Protocol) {
				list = append(list, ib)
			}
		}
		return list, nil
	}
	list := make([]*model.Inbound, 0)
	for _, ib := range inbounds {
		if !ib.Enable || !InboundSupportsLink(ib.Protocol) {
			continue
		}
		gid := ib.GameId
		if gid <= 0 {
			gid = 0
		}
		if gid == gameId {
			list = append(list, ib)
		}
	}
	return list, nil
}

func (s *SubscriptionService) CollectShareLinks(subHost string, requestHost string, gameId int) []string {
	inbounds, err := s.filterInbounds(gameId)
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

func (s *SubscriptionService) GenBase64Subscription(subHost string, requestHost string, gameId int) string {
	links := s.CollectShareLinks(subHost, requestHost, gameId)
	if len(links) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n")))
}

func (s *SubscriptionService) GenLinksText(subHost string, requestHost string, gameId int) string {
	links := s.CollectShareLinks(subHost, requestHost, gameId)
	return strings.Join(links, "\n")
}

func (s *SubscriptionService) GenClashSubscription(subHost string, requestHost string, gameId int) string {
	inbounds, err := s.filterInbounds(gameId)
	if err != nil {
		return ""
	}
	return GenClashYamlByGame(inbounds, subHost, requestHost)
}
