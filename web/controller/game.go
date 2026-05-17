package controller

import (
	"github.com/gin-gonic/gin"
	"strconv"
	"x-ui/database/model"
	"x-ui/web/service"
)

type GameController struct {
	gameService service.GameService
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

func (a *GameController) del(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "删除游戏", err)
		return
	}
	err = a.gameService.Del(id)
	jsonMsg(c, "删除游戏", err)
}
