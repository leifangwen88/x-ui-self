package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"x-ui/util/common"
	"x-ui/web/service"
	"x-ui/web/session"
)

type PanelSyncController struct {
	syncService service.PanelSyncService
}

func NewPanelSyncPublicController(g *gin.RouterGroup) *PanelSyncController {
	a := &PanelSyncController{}
	api := g.Group("/api/panel-sync")
	api.POST("/event", a.receiveEvent)
	api.GET("/outbox", a.listOutbox)
	api.GET("/snapshot", a.snapshot)
	api.POST("/align-apply", a.alignApply)
	return a
}

func NewPanelSyncManageController(g *gin.RouterGroup) *PanelSyncController {
	a := &PanelSyncController{}
	sg := g.Group("/sync")
	sg.POST("/config", a.getConfig)
	sg.POST("/saveConfig", a.saveConfig)
	sg.POST("/runNow", a.runNow)
	sg.POST("/align/compare", a.alignCompare)
	sg.POST("/align/apply", a.alignApplyLocal)
	return a
}

func (a *PanelSyncController) syncSecret(c *gin.Context) (string, bool) {
	secret := c.GetHeader("X-Panel-Sync-Secret")
	if secret == "" {
		secret = c.Query("secret")
	}
	return secret, secret != ""
}

func (a *PanelSyncController) receiveEvent(c *gin.Context) {
	secret, ok := a.syncSecret(c)
	if !ok {
		c.String(401, "missing sync secret")
		return
	}
	evt := service.SyncEventDTO{}
	if err := c.ShouldBindJSON(&evt); err != nil {
		jsonMsg(c, "接收同步事件", err)
		return
	}
	err := a.syncService.ReceiveEvent(secret, evt)
	jsonMsg(c, "接收同步事件", err)
}

func (a *PanelSyncController) listOutbox(c *gin.Context) {
	secret, ok := a.syncSecret(c)
	if !ok {
		c.String(401, "missing sync secret")
		return
	}
	cfg, err := a.syncService.GetConfig()
	if err != nil {
		jsonMsg(c, "拉取同步事件", err)
		return
	}
	if secret != cfg.Secret {
		jsonMsg(c, "拉取同步事件", common.NewError("同步密钥无效"))
		return
	}
	since, _ := strconv.ParseInt(c.Query("since"), 10, 64)
	list, err := a.syncService.ListOutboxSince(since, 500)
	if err != nil {
		jsonMsg(c, "拉取同步事件", err)
		return
	}
	jsonObj(c, list, nil)
}

func (a *PanelSyncController) snapshot(c *gin.Context) {
	secret, ok := a.syncSecret(c)
	if !ok {
		c.String(401, "missing sync secret")
		return
	}
	cfg, err := a.syncService.GetConfig()
	if err != nil {
		jsonMsg(c, "同步快照", err)
		return
	}
	if secret != cfg.Secret {
		jsonMsg(c, "同步快照", common.NewError("同步密钥无效"))
		return
	}
	snap, err := a.syncService.BuildLocalSnapshot()
	if err != nil {
		jsonMsg(c, "同步快照", err)
		return
	}
	jsonObj(c, snap, nil)
}

func (a *PanelSyncController) alignApply(c *gin.Context) {
	secret, ok := a.syncSecret(c)
	if !ok {
		c.String(401, "missing sync secret")
		return
	}
	req := &service.PanelAlignRemoteApply{}
	if err := c.ShouldBindJSON(req); err != nil {
		jsonMsg(c, "应用对齐快照", err)
		return
	}
	err := a.syncService.ReceiveAlignApply(secret, req.Snapshot, req.AlignedAt, req.OriginBaseURL, req.Scope)
	jsonMsg(c, "应用对齐快照", err)
}

func (a *PanelSyncController) getConfig(c *gin.Context) {
	cfg, err := a.syncService.GetConfig()
	if err != nil {
		jsonMsg(c, "获取同步配置", err)
		return
	}
	jsonObj(c, cfg, nil)
}

func (a *PanelSyncController) saveConfig(c *gin.Context) {
	req := &service.PanelSyncConfig{}
	if err := c.ShouldBindJSON(req); err != nil {
		jsonMsg(c, "保存同步配置", err)
		return
	}
	err := a.syncService.SaveConfig(req)
	jsonMsg(c, "保存同步配置", err)
}

func (a *PanelSyncController) runNow(c *gin.Context) {
	go a.syncService.RunSyncCycle()
	jsonObj(c, map[string]bool{"ok": true}, nil)
}

func (a *PanelSyncController) alignCompare(c *gin.Context) {
	req := struct {
		PeerIndex int `json:"peerIndex"`
	}{}
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, "对比对齐", err)
		return
	}
	res, err := a.syncService.CompareWithPeer(req.PeerIndex)
	if err != nil {
		jsonMsg(c, "对比对齐", err)
		return
	}
	jsonObj(c, res, nil)
}

func (a *PanelSyncController) alignApplyLocal(c *gin.Context) {
	req := &service.PanelAlignApplyRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, "执行首次对齐", err)
		return
	}
	user := session.GetLoginUser(c)
	res, err := a.syncService.ApplyClusterAlign(user.Id, req)
	if err != nil {
		jsonMsg(c, "执行首次对齐", err)
		return
	}
	jsonObj(c, res, nil)
}
