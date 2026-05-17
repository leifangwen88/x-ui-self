package controller

import (
	"github.com/gin-gonic/gin"
	"x-ui/web/service"
)

type SocksProxyController struct {
	socksProxyService service.SocksProxyService
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
		Text string `form:"text" json:"text"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "导入", err)
		return
	}
	result, err := a.socksProxyService.ImportFromText(req.Text)
	if err != nil {
		jsonMsg(c, "导入", err)
		return
	}
	a.xrayService.SetToNeedRestart()
	jsonObj(c, result, nil)
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
