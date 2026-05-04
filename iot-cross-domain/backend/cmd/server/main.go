package main

import (
	"log"
	"net/http"
	"strings"

	"iot-cross-domain/backend/internal/api"
	"iot-cross-domain/backend/internal/chain"
	"iot-cross-domain/backend/internal/config"
	"iot-cross-domain/backend/internal/db"
	"iot-cross-domain/backend/internal/middleware"
	"iot-cross-domain/backend/internal/model"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func main() {
	cfg := config.Load()
	conn, err := db.Open(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("open db failed: %v", err)
	}

	if err := seed(conn); err != nil {
		log.Fatalf("seed failed: %v", err)
	}

	chainSvc, err := chain.NewFabricGatewayService(cfg.Fabric)
	if err != nil {
		log.Fatalf("init fabric gateway failed: %v", err)
	}
	defer func() {
		_ = chainSvc.Close()
	}()

	h := api.NewHandler(conn, cfg.JWTSecret, chainSvc)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		} else {
			c.Header("Access-Control-Allow-Origin", "*")
		}
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", strings.Join([]string{
			"Content-Type",
			"Authorization",
			"X-Device-DID",
			"X-Target-Domain",
		}, ","))
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	router.POST("/api/auth/login", h.Login)

	auth := router.Group("/api")
	auth.Use(middleware.JWTAuth(cfg.JWTSecret))
	auth.Use(middleware.EnsureUserValid(conn))
	{
		auth.GET("/auth/me", h.Me)
		auth.GET("/domains", h.ListDomains)
		auth.POST("/domains", h.CreateDomain)
		auth.POST("/domains/with-admin", h.CreateDomainWithAdmin)
		auth.GET("/users", h.ListUsers)
		auth.POST("/users", h.CreateUser)
		auth.POST("/users/:id/revoke", h.RevokeDomainAdmin)
		auth.POST("/devices", h.CreateDevice)
		auth.GET("/devices", h.ListDevices)
		auth.GET("/devices/resolve", h.ResolveDeviceByDID)
		auth.PUT("/devices/:id", h.UpdateDevice)
		auth.DELETE("/devices/:id", h.DeleteDevice)
		auth.POST("/devices/:id/lifecycle", h.UpdateDeviceLifecycle)
		auth.GET("/trust-policies", h.ListTrustPolicies)
		auth.POST("/trust-policies", h.UpsertTrustPolicy)
		auth.DELETE("/trust-policies/:id", h.DeleteTrustPolicy)
		auth.GET("/system/oracle/nodes", h.ListOracleNodes)
		auth.POST("/system/oracle/nodes", h.RegisterOracleNode)
		auth.POST("/system/oracle/nodes/:id/rotate-key", h.RotateOracleNodeKey)
		auth.GET("/system/oracle/threshold", h.GetOracleThreshold)
		auth.POST("/system/oracle/threshold", h.SetOracleThreshold)
		auth.GET("/dashboard/overview", h.DashboardOverview)
		auth.GET("/monitor/overview", h.MonitorOverview)
		auth.GET("/monitor/devices", h.MonitorDevices)
		auth.GET("/monitor/alerts", h.MonitorAlerts)
		auth.POST("/cross/request", h.RequestCrossDomainAuth)
		auth.POST("/cross/verify", h.VerifyCrossDomainAuth)
		auth.GET("/cross/pending", h.ListPendingCrossAuth)
		auth.GET("/cross/history", h.ListMyCrossAuth)
		auth.POST("/cross/approve", h.ApproveCrossDomainAuth)
		auth.POST("/cross/revoke", h.RevokeCrossDomainAuth)
		auth.GET("/cross/status/:requestId", h.GetCrossAuthStatus)
		auth.POST("/oracle/report/:deviceDid", h.OracleReport)
		auth.GET("/oracle/check/:deviceDid", h.OracleCheck)
		auth.GET("/audit/logs", h.AuditLogs)
		auth.GET("/audit/search", h.AuditSearch)
		auth.GET("/chain/info", h.ChainInfo)
		auth.GET("/chain/topology", h.ChainTopology)
		auth.GET("/chain/blocks", h.ChainBlocks)
		auth.GET("/chain/blocks/:num", h.ChainBlockDetail)
		auth.POST("/chain/verify", h.ChainVerify)
		auth.GET("/operations/catalog", h.ListOperationCatalog)
		auth.GET("/operations/history", h.ListOperationHistory)

		protected := auth.Group("/operations")
		protected.Use(middleware.RequireCrossDomainAuth(conn))
		protected.POST("/protected", h.ProtectedOperation)
	}

	log.Printf("server listening on :%s", cfg.Port)
	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("start server failed: %v", err)
	}
}

func seed(conn *gorm.DB) error {
	var cnt int64
	if err := conn.Model(&model.User{}).Where("username = ?", "admin").Count(&cnt).Error; err != nil {
		return err
	}
	if cnt == 0 {
		hash, err := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if err := conn.Create(&model.User{
			Username:     "admin",
			PasswordHash: string(hash),
			DisplayName:  "管理员",
			Role:         "ADMIN",
			DomainCode:   "",
		}).Error; err != nil {
			return err
		}
	}

	defaultDomains := []model.Domain{
		{Code: "domain-a", Name: "制造商域"},
		{Code: "domain-b", Name: "服务商域"},
	}
	for _, d := range defaultDomains {
		_ = conn.FirstOrCreate(&model.Domain{}, model.Domain{Code: d.Code, Name: d.Name}).Error
	}
	_ = conn.FirstOrCreate(&model.DomainTrustPolicy{}, model.DomainTrustPolicy{
		FromDomainCode: "domain-a", ToDomainCode: "domain-b", PolicyLevel: "ALLOW", AllowedPerms: "READ,WRITE,ADMIN", AllowedRes: "*", Enabled: true, UpdatedBy: "system",
	}).Error
	_ = conn.FirstOrCreate(&model.DomainTrustPolicy{}, model.DomainTrustPolicy{
		FromDomainCode: "domain-b", ToDomainCode: "domain-a", PolicyLevel: "ALLOW", AllowedPerms: "READ,WRITE,ADMIN", AllowedRes: "*", Enabled: true, UpdatedBy: "system",
	}).Error
	return nil
}
