package controller

import (
	"github.com/gin-gonic/gin"
	"strconv"
	"x-ui/database/model"
	"x-ui/web/service"
)

type GameController struct {
	gameService          service.GameService
	socksRotationService service.SocksRotationService
	xrayService          service.XrayService
}

func NewGameController(g *gin.RouterGroup) *GameController {
	a := &GameController{}
	a.initRouter(g)
	return a
}

func (a *GameController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/game")
	g.POST("/list", a.list)
	g.POST("/listWithStats", a.listWithStats)
	g.POST("/add", a.add)
	g.POST("/update/:id", a.update)
	g.POST("/del/:id", a.del)
	g.POST("/rotateCheck/:id", a.rotateCheck)
	g.POST("/batchRotate/:id", a.batchRotate)
}

func (a *GameController) list(c *gin.Context) {
	list, err := a.gameService.GetAll()
	if err != nil {
		jsonMsg(c, "获取游戏", err)
		return
	}
	jsonObj(c, list, nil)
}

func (a *GameController) listWithStats(c *gin.Context) {
	list, err := a.gameService.ListWithStats()
	if err != nil {
		jsonMsg(c, "获取游戏统计", err)
		return
	}
	jsonObj(c, list, nil)
}

func (a *GameController) add(c *gin.Context) {
	game := &model.Game{}
	if err := c.ShouldBind(game); err != nil {
		jsonMsg(c, "添加游戏", err)
		return
	}
	err := a.gameService.Save(game)
	jsonMsg(c, "添加游戏", err)
}

func (a *GameController) update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "修改游戏", err)
		return
	}
	game := &model.Game{Id: id}
	if err := c.ShouldBind(game); err != nil {
		jsonMsg(c, "修改游戏", err)
		return
	}
	game.Id = id
	err = a.gameService.Save(game)
	jsonMsg(c, "修改游戏", err)
}

func (a *GameController) rotateCheck(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "轮换检查", err)
		return
	}
	result, err := a.socksRotationService.CheckGameRotate(id)
	if err != nil {
		jsonMsg(c, "轮换检查", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *GameController) batchRotate(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "批量轮换", err)
		return
	}
	req := struct {
		Items []service.GameBatchRotateItem `json:"items" form:"items"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "批量轮换", err)
		return
	}
	ok, err := a.socksRotationService.BatchRotateGame(id, req.Items)
	if err != nil {
		jsonMsg(c, "批量轮换", err)
		return
	}
	a.xrayService.SetToNeedRestart()
	jsonObj(c, map[string]int{"rotated": ok}, nil)
}

func (a *GameController) del(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "删除游戏", err)
		return
	}
	err = a.gameService.Del(id)
	jsonMsg(c, "删除游戏", err)
}
