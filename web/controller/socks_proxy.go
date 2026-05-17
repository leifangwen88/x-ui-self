package controller

import (
	"github.com/gin-gonic/gin"
	"x-ui/util/common"
	"x-ui/web/service"
)

type SocksProxyController struct {
	socksProxyService service.SocksProxyService
	socksGameService  service.SocksGameService
	settingService    service.SettingService
	xrayService       service.XrayService
}

func NewSocksProxyController(g *gin.RouterGroup) *SocksProxyController {
	a := &SocksProxyController{}
	a.initRouter(g)
	return a
}

func (a *SocksProxyController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/socks")

	g.POST("/list", a.list)
	g.POST("/import", a.importText)
	g.POST("/del", a.del)
	g.POST("/delExpired", a.delExpired)
	g.POST("/updateRemark", a.updateRemark)
	g.POST("/gameStatuses", a.gameStatuses)
	g.POST("/setGameBan", a.setGameBan)
	g.POST("/setGameMark", a.setGameMark)
	g.POST("/syncTemplate", a.syncTemplate)
}

func (a *SocksProxyController) list(c *gin.Context) {
	list, err := a.socksProxyService.GetAll()
	if err != nil {
		jsonMsg(c, "获取", err)
		return
	}
	jsonObj(c, list, nil)
}

func (a *SocksProxyController) importText(c *gin.Context) {
	req := struct {
		Text       string `form:"text" json:"text"`
		ExpiryTime int64  `form:"expiryTime" json:"expiryTime"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "导入", err)
		return
	}
	result, err := a.socksProxyService.ImportFromText(req.Text, req.ExpiryTime)
	if err != nil {
		jsonMsg(c, "导入", err)
		return
	}
	a.xrayService.SetToNeedRestart()
	jsonObj(c, result, nil)
}

func (a *SocksProxyController) updateRemark(c *gin.Context) {
	req := struct {
		Id     int    `form:"id" json:"id"`
		Remark string `form:"remark" json:"remark"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "更新备注", err)
		return
	}
	err := a.socksProxyService.UpdateRemark(req.Id, req.Remark)
	jsonMsg(c, "更新备注", err)
}

func (a *SocksProxyController) gameStatuses(c *gin.Context) {
	list, err := a.socksGameService.GetAllStatuses()
	if err != nil {
		jsonMsg(c, "获取", err)
		return
	}
	jsonObj(c, list, nil)
}

func (a *SocksProxyController) setGameMark(c *gin.Context) {
	req := struct {
		SocksProxyId int    `form:"socksProxyId" json:"socksProxyId"`
		GameId       int    `form:"gameId" json:"gameId"`
		Mark         string `form:"mark" json:"mark"`
		Note         string `form:"note" json:"note"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "更新标记", err)
		return
	}
	err := a.socksGameService.SetMark(req.SocksProxyId, req.GameId, req.Mark, req.Note)
	jsonMsg(c, "更新标记", err)
}

func (a *SocksProxyController) setGameBan(c *gin.Context) {
	req := struct {
		SocksProxyId int    `form:"socksProxyId" json:"socksProxyId"`
		GameId       int    `form:"gameId" json:"gameId"`
		Banned       bool   `form:"banned" json:"banned"`
		Note         string `form:"note" json:"note"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "更新封禁", err)
		return
	}
	err := a.socksGameService.SetBanned(req.SocksProxyId, req.GameId, req.Banned, req.Note)
	jsonMsg(c, "更新封禁", err)
}

func (a *SocksProxyController) delExpired(c *gin.Context) {
	deleted, err := a.socksProxyService.DeleteExpired()
	if err != nil {
		jsonMsg(c, "删除已过期 SOCKS", err)
		return
	}
	if deleted == 0 {
		jsonMsg(c, "没有已到期的 SOCKS5", common.NewError("没有已到期的 SOCKS5"))
		return
	}
	a.xrayService.SetToNeedRestart()
	jsonObj(c, map[string]int{"deleted": deleted}, nil)
}

func (a *SocksProxyController) del(c *gin.Context) {
	req := struct {
		Ids []int `form:"ids" json:"ids"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "删除", err)
		return
	}
	err := a.socksProxyService.DeleteByIds(req.Ids)
	jsonMsg(c, "删除", err)
	if err == nil {
		a.xrayService.SetToNeedRestart()
	}
}

func (a *SocksProxyController) syncTemplate(c *gin.Context) {
	templateConfig, err := a.settingService.GetXrayConfigTemplate()
	if err != nil {
		jsonMsg(c, "同步", err)
		return
	}
	result, err := a.socksProxyService.SyncBindingsFromTemplate(templateConfig)
	if err != nil {
		jsonMsg(c, "同步", err)
		return
	}
	a.xrayService.SetToNeedRestart()
	jsonObj(c, result, nil)
}
