package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"x-ui/web/service"
	"x-ui/web/session"
)

type BackupController struct {
	backupService service.PanelBackupService
	xrayService   service.XrayService
}

func NewBackupController(g *gin.RouterGroup) *BackupController {
	a := &BackupController{}
	a.initRouter(g)
	return a
}

func (a *BackupController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/backup")
	g.POST("/export", a.export)
	g.POST("/import", a.importBackup)
	g.POST("/preview", a.preview)
}

func (a *BackupController) export(c *gin.Context) {
	data, err := a.backupService.Export()
	if err != nil {
		jsonMsg(c, "导出备份", err)
		return
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		jsonMsg(c, "导出备份", err)
		return
	}
	filename := fmt.Sprintf("x-ui-backup-%s.json", time.Now().Format("20060102-150405"))
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.String(http.StatusOK, string(raw))
}

func (a *BackupController) preview(c *gin.Context) {
	req := struct {
		BackupJSON string `json:"backupJson" form:"backupJson"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "解析备份", err)
		return
	}
	data, err := service.ParsePanelBackupJSON([]byte(req.BackupJSON))
	if err != nil {
		jsonMsg(c, "解析备份", err)
		return
	}
	jsonObj(c, a.backupService.Summary(data), nil)
}

func (a *BackupController) importBackup(c *gin.Context) {
	req := struct {
		BackupJSON   string `json:"backupJson" form:"backupJson"`
		ResetTraffic bool   `json:"resetTraffic" form:"resetTraffic"`
		Confirm      string `json:"confirm" form:"confirm"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "导入备份", err)
		return
	}
	if req.Confirm != "IMPORT" {
		jsonMsg(c, "导入备份", fmt.Errorf("请在 confirm 字段填写 IMPORT 以确认覆盖导入"))
		return
	}
	data, err := service.ParsePanelBackupJSON([]byte(req.BackupJSON))
	if err != nil {
		jsonMsg(c, "导入备份", err)
		return
	}
	user := session.GetLoginUser(c)
	result, err := a.backupService.Import(user.Id, data, service.PanelImportOptions{
		ResetTraffic: req.ResetTraffic,
	})
	if err != nil {
		jsonMsg(c, "导入备份", err)
		return
	}
	a.xrayService.SetToNeedRestart()
	jsonObj(c, result, nil)
}
