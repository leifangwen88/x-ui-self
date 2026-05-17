package controller

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"x-ui/web/service"
)

type SubscriptionController struct {
	subService     service.SubscriptionService
	settingService service.SettingService
}

func NewSubscriptionController(g *gin.RouterGroup) *SubscriptionController {
	a := &SubscriptionController{}
	a.initRouter(g)
	return a
}

func (a *SubscriptionController) initRouter(g *gin.RouterGroup) {
	g.GET("/sub/:token", a.serve)
}

func (a *SubscriptionController) serve(c *gin.Context) {
	token := c.Param("token")
	if !a.subService.ValidateToken(token) {
		c.String(http.StatusNotFound, "Not Found")
		return
	}

	subHost, _ := a.settingService.GetSubHost()
	reqHost := hostOnlyFromRequest(c)

	subType := strings.ToLower(strings.TrimSpace(c.Query("type")))
	if subType == "" {
		subType = "base64"
	}
	gameId := -1
	if g := strings.TrimSpace(c.Query("gameId")); g != "" {
		if n, err := strconv.Atoi(g); err == nil {
			gameId = n
		}
	}

	switch subType {
	case "clash":
		body := a.subService.GenClashSubscription(subHost, reqHost, gameId)
		if body == "" {
			c.String(http.StatusNotFound, "No available nodes")
			return
		}
		c.Header("Content-Type", "application/yaml; charset=utf-8")
		c.Header("Content-Disposition", "attachment; filename=clash.yaml")
		c.String(http.StatusOK, body)
	case "links":
		body := a.subService.GenLinksText(subHost, reqHost, gameId)
		if body == "" {
			c.String(http.StatusNotFound, "No available nodes")
			return
		}
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.String(http.StatusOK, body)
	default:
		body := a.subService.GenBase64Subscription(subHost, reqHost, gameId)
		if body == "" {
			c.String(http.StatusNotFound, "No available nodes")
			return
		}
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.String(http.StatusOK, body)
	}
}

func hostOnlyFromRequest(c *gin.Context) string {
	host := c.Request.Host
	if xf := c.GetHeader("X-Forwarded-Host"); xf != "" {
		host = strings.Split(xf, ",")[0]
		host = strings.TrimSpace(host)
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}
