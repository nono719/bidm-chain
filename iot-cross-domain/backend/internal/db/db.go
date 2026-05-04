package db

import (
	"iot-cross-domain/backend/internal/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func Open(mysqlDSN string) (*gorm.DB, error) {
	conn, err := gorm.Open(mysql.Open(mysqlDSN), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := conn.AutoMigrate(
		&model.User{},
		&model.Domain{},
		&model.Device{},
		&model.CrossDomainAuthSession{},
		&model.DeviceStateUpdate{},
		&model.ProtectedOperation{},
		&model.ChainAnchor{},
		&model.AuditLog{},
		&model.DomainTrustPolicy{},
		&model.OracleNode{},
		&model.OracleSubmission{},
		&model.SystemSetting{},
	); err != nil {
		return nil, err
	}
	return conn, nil
}
