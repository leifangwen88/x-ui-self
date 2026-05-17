package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"x-ui/config"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/util/common"

	"gorm.io/gorm"
)

const PanelBackupSchemaVersion = 1

type PanelBackup struct {
	SchemaVersion int    `json:"schemaVersion"`
	ExportedAt    int64  `json:"exportedAt"`
	AppVersion    string `json:"appVersion"`

	Settings map[string]string `json:"settings"`

	Games          []GameBackup          `json:"games"`
	SocksProxies   []SocksBackup         `json:"socksProxies"`
	Inbounds       []InboundBackup       `json:"inbounds"`
	SocksGameMarks []SocksGameMarkBackup `json:"socksGameMarks"`
}

type GameBackup struct {
	Name      string `json:"name"`
	Code      string `json:"code"`
	Enable    bool   `json:"enable"`
	SortOrder int    `json:"sortOrder"`
	Remark    string `json:"remark"`
}

type SocksBackup struct {
	Address    string `json:"address"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Enable     bool   `json:"enable"`
	Remark     string `json:"remark"`
	CreatedAt  int64  `json:"createdAt"`
	ExpiryTime int64  `json:"expiryTime"`
}

type InboundBackup struct {
	Port           int    `json:"port"`
	Remark         string `json:"remark"`
	Enable         bool   `json:"enable"`
	ExpiryTime     int64  `json:"expiryTime"`
	Up             int64  `json:"up"`
	Down           int64  `json:"down"`
	Total          int64  `json:"total"`
	Listen         string `json:"listen"`
	Protocol       string `json:"protocol"`
	Settings       string `json:"settings"`
	StreamSettings string `json:"streamSettings"`
	Sniffing       string `json:"sniffing"`
	SocksKey       string `json:"socksKey"`
	GameCode       string `json:"gameCode"`
	RotationEnable bool   `json:"rotationEnable"`
	RotationPolicy string `json:"rotationPolicy"`
	LastRotatedAt  int64  `json:"lastRotatedAt"`
}

type SocksGameMarkBackup struct {
	SocksKey   string `json:"socksKey"`
	GameCode   string `json:"gameCode"`
	Status     string `json:"status"`
	BannedAt   int64  `json:"bannedAt"`
	LastUsedAt int64  `json:"lastUsedAt"`
	UseCount   int    `json:"useCount"`
	Note       string `json:"note"`
}

type PanelBackupSummary struct {
	AppVersion     string `json:"appVersion"`
	ExportedAt     int64  `json:"exportedAt"`
	GameCount      int    `json:"gameCount"`
	SocksCount     int    `json:"socksCount"`
	InboundCount   int    `json:"inboundCount"`
	GameMarkCount  int    `json:"gameMarkCount"`
	HasXrayTemplate bool `json:"hasXrayTemplate"`
}

type PanelImportOptions struct {
	ResetTraffic bool `json:"resetTraffic"`
}

type PanelImportResult struct {
	GamesCreated      int `json:"gamesCreated"`
	SocksCreated      int `json:"socksCreated"`
	InboundsCreated   int `json:"inboundsCreated"`
	GameMarksCreated  int `json:"gameMarksCreated"`
	SettingsUpdated   int `json:"settingsUpdated"`
}

type PanelBackupService struct {
	settingService SettingService
}

var panelBackupSkipSettingKeys = map[string]bool{
	"secret": true,
}

func socksKey(address string, port int) string {
	return fmt.Sprintf("%s:%d", strings.TrimSpace(address), port)
}

func (s *PanelBackupService) Export() (*PanelBackup, error) {
	db := database.GetDB()

	var settings []*model.Setting
	if err := db.Find(&settings).Error; err != nil {
		return nil, err
	}
	settingsMap := make(map[string]string)
	for _, st := range settings {
		if panelBackupSkipSettingKeys[st.Key] {
			continue
		}
		settingsMap[st.Key] = st.Value
	}

	var games []*model.Game
	if err := db.Order("sort_order asc, id asc").Find(&games).Error; err != nil {
		return nil, err
	}
	gameCodeByID := make(map[int]string, len(games))
	gameExports := make([]GameBackup, 0, len(games))
	for _, g := range games {
		code := strings.TrimSpace(g.Code)
		if code == "" {
			code = fmt.Sprintf("game_%d", g.Id)
		}
		gameCodeByID[g.Id] = code
		gameExports = append(gameExports, GameBackup{
			Name:      g.Name,
			Code:      code,
			Enable:    g.Enable,
			SortOrder: g.SortOrder,
			Remark:    g.Remark,
		})
	}

	var socksList []*model.SocksProxy
	if err := db.Order("id asc").Find(&socksList).Error; err != nil {
		return nil, err
	}
	socksKeyByID := make(map[int]string, len(socksList))
	socksExports := make([]SocksBackup, 0, len(socksList))
	for _, sp := range socksList {
		key := socksKey(sp.Address, sp.Port)
		socksKeyByID[sp.Id] = key
		socksExports = append(socksExports, SocksBackup{
			Address:    sp.Address,
			Port:       sp.Port,
			Username:   sp.Username,
			Password:   sp.Password,
			Enable:     sp.Enable,
			Remark:     sp.Remark,
			CreatedAt:  sp.CreatedAt,
			ExpiryTime: sp.ExpiryTime,
		})
	}

	var inbounds []*model.Inbound
	if err := db.Order("id asc").Find(&inbounds).Error; err != nil {
		return nil, err
	}
	inboundExports := make([]InboundBackup, 0, len(inbounds))
	for _, ib := range inbounds {
		item := InboundBackup{
			Port:           ib.Port,
			Remark:         ib.Remark,
			Enable:         ib.Enable,
			ExpiryTime:     ib.ExpiryTime,
			Up:             ib.Up,
			Down:           ib.Down,
			Total:          ib.Total,
			Listen:         ib.Listen,
			Protocol:       string(ib.Protocol),
			Settings:       ib.Settings,
			StreamSettings: ib.StreamSettings,
			Sniffing:       ib.Sniffing,
			RotationEnable: ib.RotationEnable,
			RotationPolicy: ib.RotationPolicy,
			LastRotatedAt:  ib.LastRotatedAt,
		}
		if ib.SocksProxyId > 0 {
			item.SocksKey = socksKeyByID[ib.SocksProxyId]
		}
		if ib.GameId > 0 {
			item.GameCode = gameCodeByID[ib.GameId]
		}
		inboundExports = append(inboundExports, item)
	}

	socksGame := SocksGameService{}
	statuses, err := socksGame.GetAllStatuses()
	if err != nil {
		return nil, err
	}
	markExports := make([]SocksGameMarkBackup, 0, len(statuses))
	for _, st := range statuses {
		sk := socksKeyByID[st.SocksProxyId]
		gc := gameCodeByID[st.GameId]
		if sk == "" || gc == "" {
			continue
		}
		markExports = append(markExports, SocksGameMarkBackup{
			SocksKey:   sk,
			GameCode:   gc,
			Status:     st.Status,
			BannedAt:   st.BannedAt,
			LastUsedAt: st.LastUsedAt,
			UseCount:   st.UseCount,
			Note:       st.Note,
		})
	}

	return &PanelBackup{
		SchemaVersion:  PanelBackupSchemaVersion,
		ExportedAt:     time.Now().UnixMilli(),
		AppVersion:     config.GetVersion(),
		Settings:       settingsMap,
		Games:          gameExports,
		SocksProxies:   socksExports,
		Inbounds:       inboundExports,
		SocksGameMarks: markExports,
	}, nil
}

func (s *PanelBackupService) Summary(data *PanelBackup) PanelBackupSummary {
	sum := PanelBackupSummary{
		AppVersion:    data.AppVersion,
		ExportedAt:    data.ExportedAt,
		GameCount:     len(data.Games),
		SocksCount:    len(data.SocksProxies),
		InboundCount:  len(data.Inbounds),
		GameMarkCount: len(data.SocksGameMarks),
	}
	if data.Settings != nil && strings.TrimSpace(data.Settings["xrayTemplateConfig"]) != "" {
		sum.HasXrayTemplate = true
	}
	return sum
}

func (s *PanelBackupService) Import(userId int, data *PanelBackup, opts PanelImportOptions) (*PanelImportResult, error) {
	if data == nil {
		return nil, common.NewError("备份数据为空")
	}
	if data.SchemaVersion != PanelBackupSchemaVersion {
		return nil, common.NewError("不支持的备份版本:", data.SchemaVersion)
	}

	result := &PanelImportResult{}
	db := database.GetDB()
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&model.SocksRotationLog{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&model.SocksGameStatus{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&model.Inbound{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&model.SocksProxy{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&model.Game{}).Error; err != nil {
			return err
		}

		gameIDByCode := make(map[string]int)
		for _, g := range data.Games {
			name := strings.TrimSpace(g.Name)
			if name == "" {
				return common.NewError("备份中存在空游戏名称")
			}
			code := strings.TrimSpace(g.Code)
			if code == "" {
				code = fmt.Sprintf("game_%d", time.Now().UnixNano())
			}
			if _, dup := gameIDByCode[code]; dup {
				return common.NewError("备份中游戏 code 重复:", code)
			}
			row := &model.Game{
				Name:      name,
				Code:      code,
				Enable:    g.Enable,
				SortOrder: g.SortOrder,
				Remark:    g.Remark,
			}
			if err := tx.Create(row).Error; err != nil {
				return err
			}
			gameIDByCode[code] = row.Id
			result.GamesCreated++
		}

		socksIDByKey := make(map[string]int)
		for _, sp := range data.SocksProxies {
			key := socksKey(sp.Address, sp.Port)
			if key == ":0" {
				continue
			}
			row := &model.SocksProxy{
				Address:    strings.TrimSpace(sp.Address),
				Port:       sp.Port,
				Username:   sp.Username,
				Password:   sp.Password,
				Enable:     sp.Enable,
				Remark:     sp.Remark,
				CreatedAt:  sp.CreatedAt,
				ExpiryTime: sp.ExpiryTime,
			}
			if row.CreatedAt <= 0 {
				row.CreatedAt = time.Now().UnixMilli()
			}
			if err := tx.Create(row).Error; err != nil {
				return common.NewError("导入 SOCKS 失败 ", key, ": ", err)
			}
			socksIDByKey[key] = row.Id
			result.SocksCreated++
		}

		for _, ib := range data.Inbounds {
			if ib.Port <= 0 || ib.Port > 65535 {
				return common.NewError("无效入站端口:", ib.Port)
			}
			policy := ib.RotationPolicy
			if policy == "" {
				policy = model.RotationPolicyPreferUnusedUnbanned
			}
			row := &model.Inbound{
				UserId:         userId,
				Up:             ib.Up,
				Down:           ib.Down,
				Total:          ib.Total,
				Remark:         ib.Remark,
				Enable:         ib.Enable,
				ExpiryTime:     ib.ExpiryTime,
				Listen:         ib.Listen,
				Port:           ib.Port,
				Protocol:       model.Protocol(ib.Protocol),
				Settings:       ib.Settings,
				StreamSettings: ib.StreamSettings,
				Sniffing:       ib.Sniffing,
				Tag:            fmt.Sprintf("inbound-%v", ib.Port),
				RotationEnable: ib.RotationEnable,
				RotationPolicy: policy,
				LastRotatedAt:  ib.LastRotatedAt,
			}
			if opts.ResetTraffic {
				row.Up = 0
				row.Down = 0
			}
			if ib.SocksKey != "" {
				sid, ok := socksIDByKey[ib.SocksKey]
				if !ok {
					return common.NewError("入站端口 ", ib.Port, " 引用了不存在的 SOCKS: ", ib.SocksKey)
				}
				row.SocksProxyId = sid
			}
			if ib.GameCode != "" {
				gid, ok := gameIDByCode[ib.GameCode]
				if !ok {
					return common.NewError("入站端口 ", ib.Port, " 引用了不存在的游戏: ", ib.GameCode)
				}
				row.GameId = gid
			}
			if err := tx.Create(row).Error; err != nil {
				return common.NewError("导入入站端口 ", ib.Port, " 失败: ", err)
			}
			result.InboundsCreated++
		}

		for _, mk := range data.SocksGameMarks {
			sid, ok := socksIDByKey[mk.SocksKey]
			if !ok {
				continue
			}
			gid, ok := gameIDByCode[mk.GameCode]
			if !ok {
				continue
			}
			status := mk.Status
			if status == "" {
				status = model.SocksGameStatusActive
			}
			row := &model.SocksGameStatus{
				SocksProxyId: sid,
				GameId:       gid,
				Status:       status,
				BannedAt:     mk.BannedAt,
				LastUsedAt:   mk.LastUsedAt,
				UseCount:     mk.UseCount,
				Note:         mk.Note,
			}
			if err := tx.Create(row).Error; err != nil {
				return err
			}
			result.GameMarksCreated++
		}

		for key, value := range data.Settings {
			if panelBackupSkipSettingKeys[key] {
				continue
			}
			st := &model.Setting{Key: key, Value: value}
			var exist model.Setting
			err := tx.Where("key = ?", key).First(&exist).Error
			if err == nil {
				if err := tx.Model(&exist).Update("value", value).Error; err != nil {
					return err
				}
			} else if err == gorm.ErrRecordNotFound {
				if err := tx.Create(st).Error; err != nil {
					return err
				}
			} else {
				return err
			}
			result.SettingsUpdated++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func ParsePanelBackupJSON(raw []byte) (*PanelBackup, error) {
	data := &PanelBackup{}
	if err := json.Unmarshal(raw, data); err != nil {
		return nil, common.NewError("备份 JSON 解析失败:", err)
	}
	return data, nil
}
