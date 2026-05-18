package database

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io/fs"
	"os"
	"path"
	"x-ui/config"
	"x-ui/database/model"
)

var db *gorm.DB

func initUser() error {
	err := db.AutoMigrate(&model.User{})
	if err != nil {
		return err
	}
	var count int64
	err = db.Model(&model.User{}).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		user := &model.User{
			Username: "admin",
			Password: "admin",
		}
		return db.Create(user).Error
	}
	return nil
}

func initInbound() error {
	return db.AutoMigrate(&model.Inbound{})
}

func initSocksProxy() error {
	return db.AutoMigrate(&model.SocksProxy{})
}

func initGame() error {
	return db.AutoMigrate(&model.Game{})
}

func initSocksGame() error {
	if err := db.AutoMigrate(&model.SocksGameStatus{}, &model.SocksRotationLog{}); err != nil {
		return err
	}
	return backfillSocksGameStatusKeys()
}

func backfillSocksGameStatusKeys() error {
	var list []*model.SocksGameStatus
	if err := db.Where("socks_address = '' OR socks_address IS NULL").Find(&list).Error; err != nil {
		return err
	}
	for _, st := range list {
		if st.SocksProxyId <= 0 {
			continue
		}
		var sp model.SocksProxy
		if err := db.First(&sp, st.SocksProxyId).Error; err != nil {
			continue
		}
		_ = db.Model(st).Updates(map[string]interface{}{
			"socks_address": sp.Address,
			"socks_port":    sp.Port,
		}).Error
	}
	return nil
}

func initSync() error {
	return db.AutoMigrate(&model.SyncOutbox{}, &model.SyncReceived{}, &model.SyncPeerCursor{})
}

func initSetting() error {
	return db.AutoMigrate(&model.Setting{})
}

func InitDB(dbPath string) error {
	dir := path.Dir(dbPath)
	err := os.MkdirAll(dir, fs.ModeDir)
	if err != nil {
		return err
	}

	var gormLogger logger.Interface

	if config.IsDebug() {
		gormLogger = logger.Default
	} else {
		gormLogger = logger.Discard
	}

	c := &gorm.Config{
		Logger: gormLogger,
	}
	db, err = gorm.Open(sqlite.Open(dbPath), c)
	if err != nil {
		return err
	}

	err = initUser()
	if err != nil {
		return err
	}
	err = initInbound()
	if err != nil {
		return err
	}
	err = initSetting()
	if err != nil {
		return err
	}
	err = initSocksProxy()
	if err != nil {
		return err
	}
	err = initGame()
	if err != nil {
		return err
	}
	err = initSocksGame()
	if err != nil {
		return err
	}
	err = initSync()
	if err != nil {
		return err
	}

	return nil
}

func GetDB() *gorm.DB {
	return db
}

func IsNotFound(err error) bool {
	return err == gorm.ErrRecordNotFound
}
