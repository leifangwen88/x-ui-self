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
	case "v2ray-json", "xray-json":
		body := a.subService.GenXrayJsonSubscription(subHost, reqHost, gameId)
		if body == "" {
			c.String(http.StatusNotFound, "No available nodes")
			return
		}
		setXrayJsonHeaders(c, subscriptionFilename("v2ray-json", gameId, "json"))
		c.String(http.StatusOK, body)
	case "v2ray":
		body := a.subService.GenV2raySubscription(subHost, reqHost, gameId)
		if body == "" {
			c.String(http.StatusNotFound, "No available nodes")
			return
		}
		setV2raySubscriptionHeaders(c)
		c.String(http.StatusOK, body)
	case "cluster-v2ray-json", "cluster-xray-json":
		if !a.subService.ClusterSubEnabled() {
			c.String(http.StatusNotFound, "Cluster subscription disabled")
			return
		}
		body := a.subService.GenClusterXrayJsonSubscription(subHost, reqHost, gameId)
		if body == "" {
			c.String(http.StatusNotFound, "No available cluster nodes")
			return
		}
		setXrayJsonHeaders(c, subscriptionFilename("cluster-v2ray-json", gameId, "json"))
		c.String(http.StatusOK, body)
	case "cluster-v2ray":
		if !a.subService.ClusterSubEnabled() {
			c.String(http.StatusNotFound, "Cluster subscription disabled")
			return
		}
		body := a.subService.GenClusterV2raySubscription(subHost, reqHost, gameId)
		if body == "" {
			c.String(http.StatusNotFound, "No available cluster nodes")
			return
		}
		setV2raySubscriptionHeaders(c)
		c.String(http.StatusOK, body)
	case "cluster-shadowrocket", "cluster-sr":
		if !a.subService.ClusterSubEnabled() {
			c.String(http.StatusNotFound, "Cluster subscription disabled")
			return
		}
		body := a.subService.GenClusterShadowrocketSubscription(subHost, reqHost, gameId)
		if body == "" {
			c.String(http.StatusNotFound, "No available cluster nodes")
			return
		}
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.Header("Content-Disposition", "attachment; filename="+subscriptionFilename("cluster-shadowrocket", gameId, "conf"))
		c.String(http.StatusOK, body)
	case "cluster-clash":
		if !a.subService.ClusterSubEnabled() {
			c.String(http.StatusNotFound, "Cluster subscription disabled")
			return
		}
		body := a.subService.GenClusterClashSubscription(subHost, reqHost, gameId)
		if body == "" {
			c.String(http.StatusNotFound, "No available cluster nodes")
			return
		}
		c.Header("Content-Type", "application/yaml; charset=utf-8")
		c.Header("Content-Disposition", "attachment; filename="+subscriptionFilename("cluster-clash", gameId, "yaml"))
		c.String(http.StatusOK, body)
	case "cluster":
		if !a.subService.ClusterSubEnabled() {
			c.String(http.StatusNotFound, "Cluster subscription disabled")
			return
		}
		if strings.ToLower(strings.TrimSpace(c.Query("format"))) == "links" {
			body := a.subService.GenClusterLinksText(subHost, reqHost, gameId)
			if body == "" {
				c.String(http.StatusNotFound, "No available cluster nodes")
				return
			}
			c.Header("Content-Type", "text/plain; charset=utf-8")
			c.String(http.StatusOK, body)
			return
		}
		body := a.subService.GenClusterBase64Subscription(subHost, reqHost, gameId)
		if body == "" {
			c.String(http.StatusNotFound, "No available cluster nodes")
			return
		}
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.String(http.StatusOK, body)
	case "clash":
		body := a.subService.GenClashSubscription(subHost, reqHost, gameId)
		if body == "" {
			c.String(http.StatusNotFound, "No available nodes")
			return
		}
		c.Header("Content-Type", "application/yaml; charset=utf-8")
		c.Header("Content-Disposition", "attachment; filename="+subscriptionFilename("clash", gameId, "yaml"))
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

func setV2raySubscriptionHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Profile-Update-Interval", "24")
	c.Header("Subscription-Userinfo", "upload=0; download=0; total=0; expire=0")
}

func setXrayJsonHeaders(c *gin.Context, filename string) {
	c.Header("Content-Type", "application/json; charset=utf-8")
	if filename != "" {
		c.Header("Content-Disposition", "attachment; filename="+filename)
	}
	c.Header("Profile-Update-Interval", "24")
}

func subscriptionFilename(kind string, gameId int, ext string) string {
	if gameId >= 0 {
		return "x-ui-game-" + strconv.Itoa(gameId) + "-" + kind + "." + ext
	}
	return "x-ui-" + kind + "." + ext
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
