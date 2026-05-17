package controller

import (
	"github.com/gin-gonic/gin"
	"x-ui/web/service"
)

type SubManageController struct {
	subService     service.SubscriptionService
	settingService service.SettingService
}

func NewSubManageController(g *gin.RouterGroup) *SubManageController {
	a := &SubManageController{}
	a.initRouter(g)
	return a
}

func (a *SubManageController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/sub")
	g.POST("/info", a.info)
	g.POST("/resetToken", a.resetToken)
	g.POST("/updateHost", a.updateHost)
}

func (a *SubManageController) info(c *gin.Context) {
	info, err := a.subService.GetInfo()
	if err != nil {
		jsonMsg(c, "获取订阅", err)
		return
	}
	jsonObj(c, info, nil)
}

func (a *SubManageController) resetToken(c *gin.Context) {
	token, err := a.subService.ResetToken()
	if err != nil {
		jsonMsg(c, "重置订阅", err)
		return
	}
	jsonObj(c, map[string]string{"token": token}, nil)
}

func (a *SubManageController) updateHost(c *gin.Context) {
	req := struct {
		SubHost string `form:"subHost" json:"subHost"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "更新订阅域名", err)
		return
	}
	err := a.settingService.SetSubHost(req.SubHost)
	jsonMsg(c, "更新订阅域名", err)
}
