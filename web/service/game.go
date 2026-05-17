package service

import (
	"fmt"
	"strings"
	"time"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/util/common"

	"gorm.io/gorm"
)

type GameService struct{}

func (s *GameService) GetAll() ([]*model.Game, error) {
	db := database.GetDB()
	var list []*model.Game
	err := db.Model(model.Game{}).Order("sort_order asc, id asc").Find(&list).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	return list, nil
}

func (s *GameService) GetEnabled() ([]*model.Game, error) {
	db := database.GetDB()
	var list []*model.Game
	err := db.Model(model.Game{}).Where("enable = ?", true).
		Order("sort_order asc, id asc").Find(&list).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	return list, nil
}

func (s *GameService) GetById(id int) (*model.Game, error) {
	if id <= 0 {
		return nil, common.NewError("无效的游戏 ID")
	}
	db := database.GetDB()
	game := &model.Game{}
	err := db.First(game, id).Error
	if err != nil {
		return nil, err
	}
	return game, nil
}

func (s *GameService) Save(game *model.Game) error {
	if game.Name == "" {
		return common.NewError("游戏名称不能为空")
	}
	db := database.GetDB()
	if game.Code == "" {
		game.Code = fmt.Sprintf("game_%d", time.Now().UnixMilli())
	}
	game.Code = strings.TrimSpace(game.Code)
	if game.Id > 0 {
		return db.Model(model.Game{}).Where("id = ?", game.Id).Updates(map[string]interface{}{
			"name":       game.Name,
			"code":       game.Code,
			"enable":     game.Enable,
			"sort_order": game.SortOrder,
			"remark":     game.Remark,
		}).Error
	}
	return db.Create(game).Error
}

func (s *GameService) Del(id int) error {
	db := database.GetDB()
	var inboundCount int64
	db.Model(model.Inbound{}).Where("game_id = ?", id).Count(&inboundCount)
	if inboundCount > 0 {
		return common.NewError("仍有入站绑定此游戏，无法删除")
	}
	return db.Delete(model.Game{}, id).Error
}
