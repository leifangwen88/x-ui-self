package controller

import (
	"encoding/json"
	"strconv"
	"strings"

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
	api.GET("/members", a.listMembers)
	api.POST("/align-apply", a.alignApply)
	return a
}

func NewPanelSyncManageController(g *gin.RouterGroup) *PanelSyncController {
	a := &PanelSyncController{}
	sg := g.Group("/sync")
	sg.POST("/config", a.getConfig)
	sg.POST("/saveConfig", a.saveConfig)
	sg.POST("/runNow", a.runNow)
	sg.POST("/fetchMembers", a.fetchMembers)
	sg.POST("/removeMember", a.removeMember)
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
	err := a.panelSync().ReceiveEvent(secret, evt)
	jsonMsg(c, "接收同步事件", err)
}

func (a *PanelSyncController) listOutbox(c *gin.Context) {
	secret, ok := a.syncSecret(c)
	if !ok {
		c.String(401, "missing sync secret")
		return
	}
	cfg, err := a.panelSync().GetConfig()
	if err != nil {
		jsonMsg(c, "拉取同步事件", err)
		return
	}
	if secret != cfg.Secret {
		jsonMsg(c, "拉取同步事件", common.NewError("同步密钥无效"))
		return
	}
	since, _ := strconv.ParseInt(c.Query("since"), 10, 64)
	list, err := a.panelSync().ListOutboxSince(since, 500)
	if err != nil {
		jsonMsg(c, "拉取同步事件", err)
		return
	}
	jsonObj(c, list, nil)
}

func (a *PanelSyncController) listMembers(c *gin.Context) {
	secret, ok := a.syncSecret(c)
	if !ok {
		c.String(401, "missing sync secret")
		return
	}
	cfg, err := a.panelSync().GetConfig()
	if err != nil {
		jsonMsg(c, "站群成员", err)
		return
	}
	if secret != cfg.Secret {
		jsonMsg(c, "站群成员", common.NewError("同步密钥无效"))
		return
	}
	res, err := a.panelSync().ListClusterMembers()
	if err != nil {
		jsonMsg(c, "站群成员", err)
		return
	}
	jsonObj(c, res, nil)
}

func (a *PanelSyncController) snapshot(c *gin.Context) {
	secret, ok := a.syncSecret(c)
	if !ok {
		c.String(401, "missing sync secret")
		return
	}
	cfg, err := a.panelSync().GetConfig()
	if err != nil {
		jsonMsg(c, "同步快照", err)
		return
	}
	if secret != cfg.Secret {
		jsonMsg(c, "同步快照", common.NewError("同步密钥无效"))
		return
	}
	snap, err := a.panelSync().BuildLocalSnapshot()
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
	err := a.panelSync().ReceiveAlignApply(secret, req.Snapshot, req.AlignedAt, req.OriginBaseURL, req.Scope)
	jsonMsg(c, "应用对齐快照", err)
}

func (a *PanelSyncController) panelSync() *service.PanelSyncService {
	if s := service.GetPanelSync(); s != nil {
		return s
	}
	return &a.syncService
}

func (a *PanelSyncController) getConfig(c *gin.Context) {
	cfg, err := a.panelSync().GetConfig()
	if err != nil {
		jsonMsg(c, "获取同步配置", err)
		return
	}
	jsonObj(c, cfg, nil)
}

func (a *PanelSyncController) saveConfig(c *gin.Context) {
	req := &service.PanelSyncConfig{}
	if err := c.ShouldBind(req); err != nil {
		jsonMsg(c, "保存同步配置", err)
		return
	}
	if len(req.Peers) == 0 {
		if peersJSON := strings.TrimSpace(c.PostForm("peersJson")); peersJSON != "" {
			if err := json.Unmarshal([]byte(peersJSON), &req.Peers); err != nil {
				jsonMsg(c, "保存同步配置", err)
				return
			}
		}
	}
	if len(req.Members) == 0 {
		if membersJSON := strings.TrimSpace(c.PostForm("membersJson")); membersJSON != "" {
			if err := json.Unmarshal([]byte(membersJSON), &req.Members); err != nil {
				jsonMsg(c, "保存同步配置", err)
				return
			}
		}
	}
	err := a.panelSync().SaveConfig(req)
	jsonMsg(c, "保存同步配置", err)
}

func (a *PanelSyncController) runNow(c *gin.Context) {
	go a.panelSync().RunSyncCycle()
	jsonObj(c, map[string]bool{"ok": true}, nil)
}

func (a *PanelSyncController) fetchMembers(c *gin.Context) {
	req := struct {
		SeedBaseURL string `json:"seedBaseUrl" form:"seedBaseUrl"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "同步站群成员", err)
		return
	}
	added, err := a.panelSync().FetchAndMergeMembersFromSeed(req.SeedBaseURL)
	if err != nil {
		jsonMsg(c, "同步站群成员", err)
		return
	}
	total := 0
	if res, listErr := a.panelSync().ListClusterMembers(); listErr == nil && res != nil {
		total = len(res.Members)
	}
	jsonObj(c, map[string]int{"added": added, "total": total}, nil)
}

func (a *PanelSyncController) removeMember(c *gin.Context) {
	req := struct {
		NodeId    string `json:"nodeId" form:"nodeId"`
		PublicURL string `json:"publicUrl" form:"publicUrl"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "移出站群", err)
		return
	}
	err := a.panelSync().RemoveClusterMember(req.NodeId, req.PublicURL)
	jsonMsg(c, "移出站群", err)
}

func (a *PanelSyncController) alignCompare(c *gin.Context) {
	req := struct {
		PeerIndex   int    `json:"peerIndex" form:"peerIndex"`
		PeerBaseURL string `json:"peerBaseUrl" form:"peerBaseUrl"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "对比对齐", err)
		return
	}
	res, err := a.panelSync().CompareWithPeer(req.PeerIndex, req.PeerBaseURL)
	if err != nil {
		jsonMsg(c, "对比对齐", err)
		return
	}
	jsonObj(c, res, nil)
}

func (a *PanelSyncController) alignApplyLocal(c *gin.Context) {
	req := &service.PanelAlignApplyRequest{}
	if err := c.ShouldBind(req); err != nil {
		jsonMsg(c, "执行首次对齐", err)
		return
	}
	user := session.GetLoginUser(c)
	res, err := a.panelSync().ApplyClusterAlign(user.Id, req)
	if err != nil {
		jsonMsg(c, "执行首次对齐", err)
		return
	}
	jsonObj(c, res, nil)
}
