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
	Token      string         `json:"token"`
	SubHost    string         `json:"subHost"`
	Base64Path string         `json:"base64Path"`
	LinksPath  string         `json:"linksPath"`
	ClashPath  string         `json:"clashPath"`
	Groups     []SubGameGroup `json:"groups"`
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
	groups, err := s.buildSubGroups(token)
	if err != nil {
		return nil, err
	}
	return &SubInfo{
		Token:      token,
		SubHost:    subHost,
		Base64Path: "sub/" + token,
		LinksPath:  "sub/" + token + "?type=links",
		ClashPath:  "sub/" + token + "?type=clash",
		Groups:     groups,
	}, nil
}

func (s *SubscriptionService) buildSubGroups(token string) ([]SubGameGroup, error) {
	games, err := s.gameService.GetAll()
	if err != nil {
		return nil, err
	}
	gameName := make(map[int]string)
	for _, g := range games {
		gameName[g.Id] = g.Name
	}
	inbounds, err := s.inboundService.GetAllInbounds()
	if err != nil {
		return nil, err
	}
	countByGame := make(map[int]int)
	for _, ib := range inbounds {
		if !ib.Enable || !InboundSupportsLink(ib.Protocol) {
			continue
		}
		gid := ib.GameId
		if gid <= 0 {
			gid = 0
		}
		countByGame[gid]++
	}
	var groups []SubGameGroup
	addGroup := func(gid int, name string) {
		n := countByGame[gid]
		if n == 0 {
			return
		}
		prefix := fmt.Sprintf("sub/%s", token)
		if gid > 0 {
			prefix += fmt.Sprintf("?gameId=%d", gid)
		} else {
			prefix += "?gameId=0"
		}
		groups = append(groups, SubGameGroup{
			GameId:       gid,
			GameName:     name,
			InboundCount: n,
			Base64Path:   prefix,
			LinksPath:    prefix + "&type=links",
			ClashPath:    prefix + "&type=clash",
		})
	}
	for _, g := range games {
		addGroup(g.Id, g.Name)
	}
	if countByGame[0] > 0 {
		addGroup(0, "未指定游戏")
	}
	return groups, nil
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
