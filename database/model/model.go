package model

import (
	"fmt"
	"x-ui/util/json_util"
	"x-ui/xray"
)

type Protocol string

const (
	VMess       Protocol = "vmess"
	VLESS       Protocol = "vless"
	Dokodemo    Protocol = "Dokodemo-door"
	Http        Protocol = "http"
	Trojan      Protocol = "trojan"
	Shadowsocks Protocol = "shadowsocks"
)

type User struct {
	Id       int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type Inbound struct {
	Id         int    `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	UserId     int    `json:"-"`
	Up         int64  `json:"up" form:"up"`
	Down       int64  `json:"down" form:"down"`
	Total      int64  `json:"total" form:"total"`
	Remark     string `json:"remark" form:"remark"`
	Enable     bool   `json:"enable" form:"enable"`
	ExpiryTime int64  `json:"expiryTime" form:"expiryTime"`

	// config part
	Listen         string   `json:"listen" form:"listen"`
	Port           int      `json:"port" form:"port" gorm:"unique"`
	Protocol       Protocol `json:"protocol" form:"protocol"`
	Settings       string   `json:"settings" form:"settings"`
	StreamSettings string   `json:"streamSettings" form:"streamSettings"`
	Tag            string   `json:"tag" form:"tag" gorm:"unique"`
	Sniffing       string   `json:"sniffing" form:"sniffing"`
	SocksProxyId   int      `json:"socksProxyId" form:"socksProxyId" gorm:"default:0"`

	GameId           int    `json:"gameId" form:"gameId" gorm:"default:0;index"`
	RotationEnable   bool   `json:"rotationEnable" form:"rotationEnable" gorm:"default:false"`
	RotationPolicy   string `json:"rotationPolicy" form:"rotationPolicy" gorm:"default:prefer_unused_unbanned"`
	LastRotatedAt    int64  `json:"lastRotatedAt"`
}

const (
	SocksGameStatusActive = "active"
	SocksGameStatusUsed   = "used"
	SocksGameStatusBanned = "banned"

	SocksGameMarkUsed   = "used"
	SocksGameMarkBanned = "banned"

	RotationPolicyPreferUnusedUnbanned = "prefer_unused_unbanned"
)

type Game struct {
	Id        int    `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Name      string `json:"name" form:"name" gorm:"unique"`
	Code      string `json:"code" form:"code" gorm:"uniqueIndex"`
	Enable    bool   `json:"enable" form:"enable" gorm:"default:true"`
	SortOrder int    `json:"sortOrder" form:"sortOrder" gorm:"default:0"`
	Remark    string `json:"remark" form:"remark"`
}

type SocksGameStatus struct {
	Id           int    `json:"id" gorm:"primaryKey;autoIncrement"`
	SocksProxyId int    `json:"socksProxyId" gorm:"uniqueIndex:idx_socks_game"`
	GameId       int    `json:"gameId" gorm:"uniqueIndex:idx_socks_game"`
	Status       string `json:"status" gorm:"default:active;index"`
	BannedAt     int64  `json:"bannedAt"`
	LastUsedAt   int64  `json:"lastUsedAt"`
	UseCount     int    `json:"useCount" gorm:"default:0"`
	Note         string `json:"note"`
}

type SocksRotationLog struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement"`
	InboundId   int    `json:"inboundId" gorm:"index"`
	GameId      int    `json:"gameId" gorm:"index"`
	FromSocksId int    `json:"fromSocksId"`
	ToSocksId   int    `json:"toSocksId"`
	Reason      string `json:"reason"`
	CreatedAt   int64  `json:"createdAt" gorm:"index"`
}

type SocksProxy struct {
	Id         int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Address    string `json:"address" gorm:"uniqueIndex:idx_socks_addr_port"`
	Port       int    `json:"port" gorm:"uniqueIndex:idx_socks_addr_port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Enable     bool   `json:"enable" gorm:"default:true"`
	Remark     string `json:"remark"`
	CreatedAt  int64  `json:"createdAt" gorm:"index"`
	ExpiryTime int64  `json:"expiryTime" gorm:"default:0"`
}

func (i *Inbound) GenXrayInboundConfig() *xray.InboundConfig {
	listen := i.Listen
	if listen != "" {
		listen = fmt.Sprintf("\"%v\"", listen)
	}
	return &xray.InboundConfig{
		Listen:         json_util.RawMessage(listen),
		Port:           i.Port,
		Protocol:       string(i.Protocol),
		Settings:       json_util.RawMessage(i.Settings),
		StreamSettings: json_util.RawMessage(i.StreamSettings),
		Tag:            i.Tag,
		Sniffing:       json_util.RawMessage(i.Sniffing),
	}
}

type Setting struct {
	Id    int    `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Key   string `json:"key" form:"key"`
	Value string `json:"value" form:"value"`
}
