package controller

import (
	"github.com/gin-gonic/gin"
)

type XUIController struct {
	BaseController

	inboundController    *InboundController
	settingController    *SettingController
	socksProxyController *SocksProxyController
	gameController       *GameController
}

func NewXUIController(g *gin.RouterGroup) *XUIController {
	a := &XUIController{}
	a.initRouter(g)
	return a
}

func (a *XUIController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/xui")
	g.Use(a.checkLogin)

	g.GET("/", a.index)
	g.GET("/inbounds", a.inbounds)
	g.GET("/setting", a.setting)
	g.GET("/socks", a.socks)
	g.GET("/games", a.games)

	a.inboundController = NewInboundController(g)
	a.settingController = NewSettingController(g)
	a.socksProxyController = NewSocksProxyController(g)
	a.gameController = NewGameController(g)
	NewSubManageController(g)
}

func (a *XUIController) index(c *gin.Context) {
	html(c, "index.html", "系统状态", nil)
}

func (a *XUIController) inbounds(c *gin.Context) {
	html(c, "inbounds.html", "入站列表", nil)
}

func (a *XUIController) setting(c *gin.Context) {
	html(c, "setting.html", "设置", nil)
}

func (a *XUIController) socks(c *gin.Context) {
	html(c, "socks.html", "SOCKS5 池", nil)
}

func (a *XUIController) games(c *gin.Context) {
	html(c, "games.html", "游戏管理", nil)
}
