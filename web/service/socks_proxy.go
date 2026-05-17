package service

import (
	"strings"
	"time"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/util/common"

	"gorm.io/gorm"
)

type SocksProxyService struct{}

type SocksImportResult struct {
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors"`
}

func (s *SocksProxyService) GetAll() ([]*model.SocksProxy, error) {
	db := database.GetDB()
	var list []*model.SocksProxy
	err := db.Model(model.SocksProxy{}).Order("id asc").Find(&list).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	return list, nil
}

func (s *SocksProxyService) GetById(id int) (*model.SocksProxy, error) {
	db := database.GetDB()
	socks := &model.SocksProxy{}
	err := db.First(socks, id).Error
	if err != nil {
		return nil, err
	}
	return socks, nil
}

func (s *SocksProxyService) GetMapByIds(ids []int) (map[int]*model.SocksProxy, error) {
	result := make(map[int]*model.SocksProxy)
	if len(ids) == 0 {
		return result, nil
	}
	db := database.GetDB()
	var list []*model.SocksProxy
	err := db.Where("id IN ?", ids).Find(&list).Error
	if err != nil {
		return nil, err
	}
	for _, item := range list {
		result[item.Id] = item
	}
	return result, nil
}

func (s *SocksProxyService) ImportFromText(text string, expiryTime int64) (*SocksImportResult, error) {
	lines := strings.Split(text, "\n")
	result := &SocksImportResult{}
	db := database.GetDB()
	now := time.Now().UnixMilli()

	for _, line := range lines {
		socks, err := parseSocksProxyLine(line)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			continue
		}
		if socks == nil {
			continue
		}
		socks.CreatedAt = now
		socks.ExpiryTime = expiryTime

		var exist model.SocksProxy
		err = db.Where("address = ? AND port = ?", socks.Address, socks.Port).First(&exist).Error
		if err == nil {
			result.Skipped++
			continue
		}
		if err != nil && err != gorm.ErrRecordNotFound {
			return nil, err
		}

		err = db.Create(socks).Error
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			continue
		}
		result.Imported++
	}
	return result, nil
}

func (s *SocksProxyService) IsExpired(socks *model.SocksProxy) bool {
	if socks == nil || socks.ExpiryTime <= 0 {
		return false
	}
	return socks.ExpiryTime < time.Now().UnixMilli()
}

func (s *SocksProxyService) DeleteByIds(ids []int) error {
	if len(ids) == 0 {
		return common.NewError("未选择要删除的 SOCKS")
	}
	db := database.GetDB()
	tx := db.Begin()
	if err := tx.Model(model.Inbound{}).Where("socks_proxy_id IN ?", ids).Update("socks_proxy_id", 0).Error; err != nil {
		tx.Rollback()
		return err
	}
	// 保留 SocksGameStatus，供游戏管理统计历史封禁数（删除 SOCKS 不减少被封禁计数）
	if err := tx.Delete(&model.SocksProxy{}, ids).Error; err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

func (s *SocksProxyService) ListExpiredIds() ([]int, error) {
	now := time.Now().UnixMilli()
	db := database.GetDB()
	var ids []int
	err := db.Model(model.SocksProxy{}).
		Where("expiry_time > 0 AND expiry_time < ?", now).
		Pluck("id", &ids).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	return ids, nil
}

func (s *SocksProxyService) DeleteExpired() (int, error) {
	ids, err := s.ListExpiredIds()
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	if err := s.DeleteByIds(ids); err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (s *SocksProxyService) UpdateEnable(id int, enable bool) error {
	db := database.GetDB()
	return db.Model(model.SocksProxy{}).Where("id = ?", id).Update("enable", enable).Error
}

func (s *SocksProxyService) UpdateRemark(id int, remark string) error {
	if id <= 0 {
		return common.NewError("无效的 SOCKS ID")
	}
	db := database.GetDB()
	result := db.Model(model.SocksProxy{}).Where("id = ?", id).Update("remark", remark)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return common.NewError("SOCKS5 不存在:", id)
	}
	return nil
}
