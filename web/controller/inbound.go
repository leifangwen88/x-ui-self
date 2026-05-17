package controller

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"strconv"
	"x-ui/database/model"
	"x-ui/logger"
	"x-ui/web/global"
	"x-ui/web/service"
	"x-ui/web/session"
)

type InboundController struct {
	inboundService      service.InboundService
	socksRotationService service.SocksRotationService
	xrayService         service.XrayService
}

func NewInboundController(g *gin.RouterGroup) *InboundController {
	a := &InboundController{}
	a.initRouter(g)
	a.startTask()
	return a
}

func (a *InboundController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/inbound")

	g.POST("/list", a.getInbounds)
	g.POST("/add", a.addInbound)
	g.POST("/del/:id", a.delInbound)
	g.POST("/update/:id", a.updateInbound)
	g.POST("/resetAllTraffic", a.resetAllTraffic)
	g.POST("/updateSocks/:id", a.updateSocks)
	g.POST("/rotate/:id", a.rotate)
}

func (a *InboundController) startTask() {
	webServer := global.GetWebServer()
	c := webServer.GetCron()
	c.AddFunc("@every 10s", func() {
		if a.xrayService.IsNeedRestartAndSetFalse() {
			err := a.xrayService.RestartXray(false)
			if err != nil {
				logger.Error("restart xray failed:", err)
			}
		}
	})
}

func (a *InboundController) getInbounds(c *gin.Context) {
	user := session.GetLoginUser(c)
	inbounds, err := a.inboundService.GetInbounds(user.Id)
	if err != nil {
		jsonMsg(c, "获取", err)
		return
	}
	jsonObj(c, inbounds, nil)
}

func (a *InboundController) addInbound(c *gin.Context) {
	inbound := &model.Inbound{}
	err := c.ShouldBind(inbound)
	if err != nil {
		jsonMsg(c, "添加", err)
		return
	}
	user := session.GetLoginUser(c)
	inbound.UserId = user.Id
	inbound.Enable = true
	inbound.Tag = fmt.Sprintf("inbound-%v", inbound.Port)
	err = a.inboundService.AddInbound(inbound)
	jsonMsg(c, "添加", err)
	if err == nil {
		a.xrayService.SetToNeedRestart()
	}
}

func (a *InboundController) delInbound(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "删除", err)
		return
	}
	err = a.inboundService.DelInbound(id)
	jsonMsg(c, "删除", err)
	if err == nil {
		a.xrayService.SetToNeedRestart()
	}
}

func (a *InboundController) updateSocks(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "修改 SOCKS 绑定", err)
		return
	}
	req := struct {
		SocksProxyId int `form:"socksProxyId" json:"socksProxyId"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "修改 SOCKS 绑定", err)
		return
	}
	err = a.inboundService.UpdateSocksProxyId(id, req.SocksProxyId)
	jsonMsg(c, "修改 SOCKS 绑定", err)
	if err == nil {
		a.xrayService.SetToNeedRestart()
	}
}

func (a *InboundController) rotate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "轮换 IP", err)
		return
	}
	req := struct {
		OutgoingMark string `form:"outgoingMark" json:"outgoingMark"`
		Reason       string `form:"reason" json:"reason"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "轮换 IP", err)
		return
	}
	result, err := a.socksRotationService.RotateInbound(id, req.OutgoingMark, req.Reason)
	if err != nil {
		jsonMsg(c, "轮换 IP", err)
		return
	}
	a.xrayService.SetToNeedRestart()
	jsonObj(c, result, nil)
}

func (a *InboundController) resetAllTraffic(c *gin.Context) {
	err := a.inboundService.ResetAllTraffic()
	jsonMsg(c, "批量重置流量", err)
}

func (a *InboundController) updateInbound(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "修改", err)
		return
	}
	inbound := &model.Inbound{
		Id: id,
	}
	err = c.ShouldBind(inbound)
	if err != nil {
		jsonMsg(c, "修改", err)
		return
	}
	err = a.inboundService.UpdateInbound(inbound)
	jsonMsg(c, "修改", err)
	if err == nil {
		a.xrayService.SetToNeedRestart()
	}
}
