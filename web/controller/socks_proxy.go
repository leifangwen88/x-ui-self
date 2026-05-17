package controller

import (
	"github.com/gin-gonic/gin"
	"x-ui/web/service"
)

type SocksProxyController struct {
	socksProxyService service.SocksProxyService
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
	g.POST("/updateRemark", a.updateRemark)
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
