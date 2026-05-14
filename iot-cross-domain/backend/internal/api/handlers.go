package api

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"iot-cross-domain/backend/internal/chain"
	"iot-cross-domain/backend/internal/middleware"
	"iot-cross-domain/backend/internal/model"
	"iot-cross-domain/backend/internal/pkgutil"
	"iot-cross-domain/backend/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Handler struct {
	DB        *gorm.DB
	JWTSecret string
	Chain     chain.Service
}

func NewHandler(db *gorm.DB, jwtSecret string, chainSvc chain.Service) *Handler {
	return &Handler{DB: db, JWTSecret: jwtSecret, Chain: chainSvc}
}

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	var u model.User
	if err := h.DB.Where("username = ?", req.Username).First(&u).Error; err != nil {
		response.Unauthorized(c, "用户名或密码错误")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
		response.Unauthorized(c, "用户名或密码错误")
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userId":      u.ID,
		"username":    u.Username,
		"displayName": u.DisplayName,
		"role":        u.Role,
		"domainCode":  u.DomainCode,
		"exp":         time.Now().Add(12 * time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(h.JWTSecret))
	if err != nil {
		response.InternalError(c, "token sign failed")
		return
	}
	response.OK(c, gin.H{"token": signed, "user": gin.H{"id": u.ID, "username": u.Username, "displayName": u.DisplayName, "role": u.Role, "domainCode": u.DomainCode}})
}

func (h *Handler) Me(c *gin.Context) {
	username := c.GetString("username")
	var u model.User
	if err := h.DB.Where("username = ?", username).First(&u).Error; err != nil {
		response.Unauthorized(c, "用户不存在")
		return
	}
	response.OK(c, gin.H{"id": u.ID, "username": u.Username, "displayName": u.DisplayName, "role": u.Role, "domainCode": u.DomainCode})
}

func (h *Handler) ListDomains(c *gin.Context) {
	var domains []model.Domain
	if err := h.DB.Order("id asc").Find(&domains).Error; err != nil {
		response.InternalError(c, "query failed")
		return
	}
	response.OK(c, domains)
}

type createUserReq struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role" binding:"required"`
	DomainCode  string `json:"domainCode"`
}

func (h *Handler) CreateUser(c *gin.Context) {
	if c.GetString("role") != "ADMIN" {
		response.Forbidden(c, "permission denied")
		return
	}
	var req createUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	role := strings.ToUpper(strings.TrimSpace(req.Role))
	if role != "ADMIN" && role != "DOMAIN_ADMIN" {
		response.BadRequest(c, "invalid role")
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		response.BadRequest(c, "invalid username")
		return
	}
	domain := strings.TrimSpace(req.DomainCode)
	if role == "DOMAIN_ADMIN" && domain == "" {
		response.BadRequest(c, "domainCode required for DOMAIN_ADMIN")
		return
	}
	if role == "ADMIN" {
		domain = ""
	}
	if role == "DOMAIN_ADMIN" {
		var d model.Domain
		if err := h.DB.Where("code = ?", domain).First(&d).Error; err != nil {
			response.BadRequest(c, "domain not found")
			return
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		response.InternalError(c, "hash failed")
		return
	}
	name := strings.TrimSpace(req.DisplayName)
	if name == "" {
		name = username
	}
	user := model.User{Username: username, PasswordHash: string(hash), DisplayName: name, Role: role, DomainCode: domain}
	if err := h.DB.Create(&user).Error; err != nil {
		response.BadRequest(c, "user exists or invalid")
		return
	}
	_ = h.auditEx(model.AuditLog{Module: "user", Action: "create", Operator: c.GetString("username"), Result: "OK", SubjectDID: "", Message: username, DetailJSON: mustJSON(gin.H{"userId": user.ID, "role": user.Role, "domainCode": user.DomainCode})})
	response.OK(c, gin.H{"id": user.ID, "username": user.Username, "displayName": user.DisplayName, "role": user.Role, "domainCode": user.DomainCode, "createdAt": user.CreatedAt})
}

func (h *Handler) ListUsers(c *gin.Context) {
	if c.GetString("role") != "ADMIN" {
		response.Forbidden(c, "permission denied")
		return
	}
	var users []model.User
	if err := h.DB.Order("id desc").Find(&users).Error; err != nil {
		response.InternalError(c, "query failed")
		return
	}
	rows := make([]gin.H, 0, len(users))
	for _, u := range users {
		rows = append(rows, gin.H{"id": u.ID, "username": u.Username, "displayName": u.DisplayName, "role": u.Role, "domainCode": u.DomainCode, "createdAt": u.CreatedAt})
	}
	response.OK(c, rows)
}

func (h *Handler) RevokeDomainAdmin(c *gin.Context) {
	if c.GetString("role") != "ADMIN" {
		response.Forbidden(c, "permission denied")
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.BadRequest(c, "invalid user id")
		return
	}

	var user model.User
	if err := h.DB.Where("id = ?", id).First(&user).Error; err != nil {
		response.BadRequest(c, "user not found")
		return
	}
	if user.Role != "DOMAIN_ADMIN" {
		response.BadRequest(c, "only DOMAIN_ADMIN can be revoked")
		return
	}

	if err := h.DB.Delete(&user).Error; err != nil {
		response.InternalError(c, "revoke failed")
		return
	}

	_ = h.auditEx(model.AuditLog{
		Module:     "user",
		Action:     "revoke_domain_admin",
		Operator:   c.GetString("username"),
		Result:     "OK",
		SubjectDID: "",
		Message:    user.Username,
		DetailJSON: mustJSON(gin.H{"userId": user.ID, "username": user.Username, "domainCode": user.DomainCode}),
	})
	response.OK(c, gin.H{"id": user.ID, "username": user.Username, "domainCode": user.DomainCode, "revoked": true})
}

type createDomainReq struct {
	Code string `json:"code" binding:"required"`
	Name string `json:"name" binding:"required"`
}

type createDomainWithAdminReq struct {
	Code          string `json:"code" binding:"required"`
	Name          string `json:"name" binding:"required"`
	AdminUsername string `json:"adminUsername" binding:"required"`
	AdminPassword string `json:"adminPassword" binding:"required"`
	AdminName     string `json:"adminName"`
}

func (h *Handler) CreateDomain(c *gin.Context) {
	if c.GetString("role") != "ADMIN" {
		response.Forbidden(c, "permission denied")
		return
	}
	var req createDomainReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	d := model.Domain{Code: req.Code, Name: req.Name}
	if err := h.DB.Create(&d).Error; err != nil {
		response.BadRequest(c, "domain already exists or invalid")
		return
	}
	_ = h.audit("domain", "create", c.GetString("username"), "OK", "")
	response.OK(c, d)
}

func (h *Handler) CreateDomainWithAdmin(c *gin.Context) {
	if c.GetString("role") != "ADMIN" {
		response.Forbidden(c, "permission denied")
		return
	}

	var req createDomainWithAdminReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}

	domainCode := strings.TrimSpace(req.Code)
	domainName := strings.TrimSpace(req.Name)
	adminUsername := strings.TrimSpace(req.AdminUsername)
	adminPassword := strings.TrimSpace(req.AdminPassword)
	adminName := strings.TrimSpace(req.AdminName)
	if domainCode == "" || domainName == "" || adminUsername == "" || adminPassword == "" {
		response.BadRequest(c, "missing required fields")
		return
	}
	if len(adminPassword) < 6 {
		response.BadRequest(c, "adminPassword too short")
		return
	}
	if adminName == "" {
		adminName = adminUsername
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
	if err != nil {
		response.InternalError(c, "hash failed")
		return
	}

	tx := h.DB.Begin()
	domain := model.Domain{Code: domainCode, Name: domainName}
	if err := tx.Create(&domain).Error; err != nil {
		_ = tx.Rollback().Error
		response.BadRequest(c, "domain already exists or invalid")
		return
	}

	user := model.User{
		Username:     adminUsername,
		PasswordHash: string(hash),
		DisplayName:  adminName,
		Role:         "DOMAIN_ADMIN",
		DomainCode:   domainCode,
	}
	if err := tx.Create(&user).Error; err != nil {
		_ = tx.Rollback().Error
		response.BadRequest(c, "admin user already exists or invalid")
		return
	}

	if err := tx.Commit().Error; err != nil {
		_ = tx.Rollback().Error
		response.InternalError(c, "create failed")
		return
	}

	_ = h.auditEx(model.AuditLog{
		Module:     "domain",
		Action:     "create_with_admin",
		Operator:   c.GetString("username"),
		Result:     "OK",
		SubjectDID: "",
		Message:    domainCode,
		DetailJSON: mustJSON(gin.H{
			"domain": gin.H{"code": domain.Code, "name": domain.Name},
			"admin":  gin.H{"username": user.Username, "displayName": user.DisplayName, "role": user.Role, "domainCode": user.DomainCode},
		}),
	})

	response.OK(c, gin.H{
		"domain": gin.H{
			"id":   domain.ID,
			"code": domain.Code,
			"name": domain.Name,
		},
		"admin": gin.H{
			"id":          user.ID,
			"username":    user.Username,
			"displayName": user.DisplayName,
			"role":        user.Role,
			"domainCode":  user.DomainCode,
		},
	})
}

type createDeviceReq struct {
	DeviceDID    string `json:"deviceDid" binding:"required"`
	DomainCode   string `json:"domainCode" binding:"required"`
	DisplayName  string `json:"displayName" binding:"required"`
	Credential   string `json:"credential" binding:"required"`
	DeviceType   string `json:"deviceType"`
	PublicKeyJWK string `json:"publicKeyJwk"`
	MetadataJSON string `json:"metadataJson"`
}

func (h *Handler) CreateDevice(c *gin.Context) {
	var req createDeviceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	role := c.GetString("role")
	userDomain := c.GetString("domainCode")
	domainCode := strings.TrimSpace(req.DomainCode)
	if role != "ADMIN" {
		domainCode = userDomain
	}
	if strings.TrimSpace(domainCode) == "" {
		response.BadRequest(c, "domainCode required")
		return
	}
	device := model.Device{
		DeviceDID:    strings.TrimSpace(req.DeviceDID),
		DomainCode:   domainCode,
		DisplayName:  req.DisplayName,
		Credential:   req.Credential,
		DeviceType:   strings.TrimSpace(req.DeviceType),
		PublicKeyJWK: strings.TrimSpace(req.PublicKeyJWK),
		MetadataJSON: strings.TrimSpace(req.MetadataJSON),
		Lifecycle:    "ACTIVE",
		RuntimeState: "UNKNOWN",
	}
	if device.DeviceType == "" {
		device.DeviceType = "传感器"
	}
	if !isValidDeviceType(device.DeviceType) {
		response.BadRequest(c, "invalid deviceType")
		return
	}
	if !isValidDeviceDID(device.DeviceDID, device.DomainCode, device.DeviceType) {
		response.BadRequest(c, "invalid deviceDid format, expected did:iot:<domain>:<type>:<id>")
		return
	}

	tx := h.DB.Begin()
	if err := tx.Create(&device).Error; err != nil {
		_ = tx.Rollback().Error
		response.BadRequest(c, "device exists or invalid")
		return
	}

	digestRaw := fmt.Sprintf("%s|%s|%s|%s", device.DeviceDID, device.DomainCode, device.DeviceType, device.MetadataJSON)
	sum := sha256.Sum256([]byte(digestRaw))
	digest := hex.EncodeToString(sum[:])
	anchor, err := h.Chain.Anchor("device_register", device.DeviceDID, digest)
	if err != nil {
		_ = tx.Rollback().Error
		response.InternalError(c, "fabric anchor failed")
		return
	}
	_ = tx.Create(&model.ChainAnchor{
		BizType:     "device_register",
		BizRef:      device.DeviceDID,
		Digest:      digest,
		TxHash:      anchor.TxHash,
		BlockHeight: anchor.BlockHeight,
	}).Error
	_ = tx.Commit().Error

	_ = h.auditEx(model.AuditLog{
		Module:      "device",
		Action:      "register",
		Operator:    c.GetString("username"),
		Result:      "OK",
		Contract:    "anchorcc",
		Method:      "AnchorRecord",
		SubjectDID:  req.DeviceDID,
		TxHash:      anchor.TxHash,
		BlockHeight: anchor.BlockHeight,
		GasUsed:     0,
		Message:     "device_register",
		DetailJSON:  mustJSON(gin.H{"device": device, "digest": digest}),
	})

	response.OK(c, gin.H{"device": device, "anchor": anchor})
}

func (h *Handler) ResolveDeviceByDID(c *gin.Context) {
	deviceDID := strings.TrimSpace(c.Query("did"))
	if deviceDID == "" {
		response.BadRequest(c, "missing did")
		return
	}
	var device model.Device
	if err := h.DB.Where("device_d_id = ?", deviceDID).First(&device).Error; err != nil {
		response.BadRequest(c, "device not found")
		return
	}
	if c.GetString("role") != "ADMIN" && device.DomainCode != c.GetString("domainCode") {
		response.Forbidden(c, "permission denied")
		return
	}

	var anchor model.ChainAnchor
	anchorOK := h.DB.Where("biz_type = ? AND biz_ref = ?", "device_register", device.DeviceDID).Order("id desc").First(&anchor).Error == nil
	resp := gin.H{
		"deviceDid":     device.DeviceDID,
		"domainCode":    device.DomainCode,
		"displayName":   device.DisplayName,
		"deviceType":    device.DeviceType,
		"publicKeyJwk":  device.PublicKeyJWK,
		"lifecycle":     device.Lifecycle,
		"runtimeState":  device.RuntimeState,
		"registerAt":    device.CreatedAt,
		"lastUpdatedAt": device.UpdatedAt,
	}
	if anchorOK {
		resp["registerTxHash"] = anchor.TxHash
		resp["registerBlockHeight"] = anchor.BlockHeight
	}
	response.OK(c, resp)
}

func (h *Handler) ListDevices(c *gin.Context) {
	var devices []model.Device
	query := h.DB.Order("id desc")
	keyword := strings.TrimSpace(c.Query("keyword"))
	deviceType := strings.TrimSpace(c.Query("deviceType"))
	if c.GetString("role") != "ADMIN" {
		query = query.Where("domain_code = ?", c.GetString("domainCode"))
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("device_d_id LIKE ? OR display_name LIKE ?", like, like)
	}
	if deviceType != "" && deviceType != "全部" {
		query = query.Where("device_type = ?", deviceType)
	}
	if err := query.Find(&devices).Error; err != nil {
		response.InternalError(c, "query failed")
		return
	}
	response.OK(c, devices)
}

type updateDeviceReq struct {
	DisplayName  string `json:"displayName" binding:"required"`
	DeviceType   string `json:"deviceType" binding:"required"`
	MetadataJSON string `json:"metadataJson"`
}

func (h *Handler) UpdateDevice(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.BadRequest(c, "invalid device id")
		return
	}
	var req updateDeviceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	if !isValidDeviceType(req.DeviceType) {
		response.BadRequest(c, "invalid deviceType")
		return
	}
	var device model.Device
	if err := h.DB.Where("id = ?", id).First(&device).Error; err != nil {
		response.BadRequest(c, "device not found")
		return
	}
	if c.GetString("role") != "ADMIN" && device.DomainCode != c.GetString("domainCode") {
		response.Forbidden(c, "permission denied")
		return
	}
	device.DisplayName = strings.TrimSpace(req.DisplayName)
	device.DeviceType = strings.TrimSpace(req.DeviceType)
	device.MetadataJSON = strings.TrimSpace(req.MetadataJSON)
	if err := h.DB.Save(&device).Error; err != nil {
		response.InternalError(c, "update failed")
		return
	}
	_ = h.auditEx(model.AuditLog{
		Module:     "device",
		Action:     "update",
		Operator:   c.GetString("username"),
		Result:     "OK",
		SubjectDID: device.DeviceDID,
		Message:    "device updated",
		DetailJSON: mustJSON(gin.H{"id": device.ID, "displayName": device.DisplayName, "deviceType": device.DeviceType}),
	})
	response.OK(c, device)
}

func (h *Handler) DeleteDevice(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.BadRequest(c, "invalid device id")
		return
	}
	var device model.Device
	if err := h.DB.Where("id = ?", id).First(&device).Error; err != nil {
		response.BadRequest(c, "device not found")
		return
	}
	if c.GetString("role") != "ADMIN" && device.DomainCode != c.GetString("domainCode") {
		response.Forbidden(c, "permission denied")
		return
	}
	if err := h.DB.Delete(&device).Error; err != nil {
		response.InternalError(c, "delete failed")
		return
	}
	_ = h.auditEx(model.AuditLog{
		Module:     "device",
		Action:     "delete",
		Operator:   c.GetString("username"),
		Result:     "OK",
		SubjectDID: device.DeviceDID,
		Message:    "device deleted",
	})
	response.OK(c, gin.H{"id": device.ID, "deviceDid": device.DeviceDID})
}

type lifecycleReq struct {
	Action string `json:"action" binding:"required"`
}

func (h *Handler) UpdateDeviceLifecycle(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.BadRequest(c, "invalid device id")
		return
	}
	var req lifecycleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	action := strings.ToUpper(strings.TrimSpace(req.Action))
	if action != "ACTIVATE" && action != "FREEZE" && action != "REVOKE" && action != "REACTIVATE" {
		response.BadRequest(c, "invalid action")
		return
	}
	var device model.Device
	if err := h.DB.Where("id = ?", id).First(&device).Error; err != nil {
		response.BadRequest(c, "device not found")
		return
	}
	if c.GetString("role") != "ADMIN" && device.DomainCode != c.GetString("domainCode") {
		response.Forbidden(c, "permission denied")
		return
	}

	switch action {
	case "ACTIVATE", "REACTIVATE":
		device.Lifecycle = "ACTIVE"
	case "FREEZE":
		device.Lifecycle = "FROZEN"
	case "REVOKE":
		device.Lifecycle = "REVOKED"
	}
	if err := h.DB.Save(&device).Error; err != nil {
		response.InternalError(c, "update lifecycle failed")
		return
	}
	_ = h.auditEx(model.AuditLog{
		Module:     "device",
		Action:     "lifecycle_" + strings.ToLower(action),
		Operator:   c.GetString("username"),
		Result:     "OK",
		SubjectDID: device.DeviceDID,
		Message:    device.Lifecycle,
		DetailJSON: mustJSON(gin.H{"deviceId": device.ID, "lifecycle": device.Lifecycle}),
	})
	response.OK(c, gin.H{"id": device.ID, "deviceDid": device.DeviceDID, "lifecycle": device.Lifecycle})
}

type crossRequestReq struct {
	DeviceDID    string `json:"deviceDid" binding:"required"`
	ToDomainCode string `json:"toDomainCode" binding:"required"`
	Resource     string `json:"resource"`
	Permission   string `json:"permission"`
	TTLSeconds   int    `json:"ttlSeconds"`
}

func (h *Handler) RequestCrossDomainAuth(c *gin.Context) {
	var req crossRequestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}

	var device model.Device
	if err := h.DB.Where("device_d_id = ?", req.DeviceDID).First(&device).Error; err != nil {
		response.BadRequest(c, "device not found")
		return
	}
	if c.GetString("role") != "ADMIN" {
		if device.DomainCode != c.GetString("domainCode") {
			response.Forbidden(c, "device not in your domain")
			return
		}
	}
	if strings.ToUpper(device.Lifecycle) != "ACTIVE" {
		response.Forbidden(c, "device lifecycle is not ACTIVE")
		return
	}
	if strings.TrimSpace(req.ToDomainCode) == "" {
		response.BadRequest(c, "toDomainCode required")
		return
	}
	okOracle, reason, detail := h.checkOracleForAuth(device.DeviceDID)
	if !okOracle {
		response.Forbidden(c, "oracle check denied: "+reason)
		return
	}
	if device.DomainCode != strings.TrimSpace(req.ToDomainCode) {
		if ok, reason := h.checkTrustPolicy(device.DomainCode, strings.TrimSpace(req.ToDomainCode), strings.TrimSpace(req.Permission), strings.TrimSpace(req.Resource)); !ok {
			response.Forbidden(c, "trust policy denied: "+reason)
			return
		}
	}

	requestID := fmt.Sprintf("REQ-%d-%d", time.Now().Unix(), rand.Intn(10000))
	challenge := pkgutil.RandomHex(16)
	ttl := req.TTLSeconds
	if ttl <= 0 {
		ttl = 1800
	}
	session := model.CrossDomainAuthSession{
		RequestID:      requestID,
		DeviceDID:      req.DeviceDID,
		FromDomainCode: device.DomainCode,
		ToDomainCode:   req.ToDomainCode,
		Resource:       strings.TrimSpace(req.Resource),
		Permission:     strings.TrimSpace(req.Permission),
		TTLSeconds:     ttl,
		Challenge:      challenge,
		Status:         "PENDING_SIGNATURE",
		RequestedBy:    c.GetString("username"),
	}
	if err := h.DB.Create(&session).Error; err != nil {
		response.InternalError(c, "create session failed")
		return
	}

	_ = h.auditEx(model.AuditLog{
		Module:     "cross_auth",
		Action:     "request",
		Operator:   c.GetString("username"),
		Result:     "OK",
		Contract:   "",
		Method:     "requestCrossDomainAuth",
		SubjectDID: req.DeviceDID,
		Message:    requestID,
		DetailJSON: mustJSON(gin.H{"session": session}),
	})
	response.OK(c, gin.H{
		"requestId": requestID,
		"challenge": challenge,
		"hint":      "signature = sha256(challenge:credential)",
		"oracle":    detail,
	})
}

type crossVerifyReq struct {
	RequestID string `json:"requestId" binding:"required"`
	Signature string `json:"signature" binding:"required"`
}

func (h *Handler) VerifyCrossDomainAuth(c *gin.Context) {
	var req crossVerifyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}

	var session model.CrossDomainAuthSession
	if err := h.DB.Where("request_id = ?", req.RequestID).First(&session).Error; err != nil {
		response.BadRequest(c, "request not found")
		return
	}
	if session.Status == "VERIFIED" {
		response.OK(c, gin.H{"requestId": req.RequestID, "status": "VERIFIED"})
		return
	}
	if session.Status == "REJECTED" {
		response.Forbidden(c, "request rejected")
		return
	}

	var device model.Device
	if err := h.DB.Where("device_d_id = ?", session.DeviceDID).First(&device).Error; err != nil {
		response.BadRequest(c, "device not found")
		return
	}
	if c.GetString("role") != "ADMIN" {
		if device.DomainCode != c.GetString("domainCode") {
			response.Forbidden(c, "device not in your domain")
			return
		}
	}

	expectRaw := session.Challenge + ":" + device.Credential
	sum := sha256.Sum256([]byte(expectRaw))
	expect := hex.EncodeToString(sum[:])
	if req.Signature != expect {
		_ = h.auditEx(model.AuditLog{
			Module:     "cross_auth",
			Action:     "verify",
			Operator:   c.GetString("username"),
			Result:     "DENY",
			Contract:   "",
			Method:     "verifyCredential",
			SubjectDID: session.DeviceDID,
			Message:    req.RequestID,
			DetailJSON: mustJSON(gin.H{"reason": "signature verify failed"}),
		})
		response.Forbidden(c, "signature verify failed")
		return
	}

	now := time.Now()
	session.SignatureVerifiedAt = &now
	_ = h.auditEx(model.AuditLog{
		Module:     "cross_auth",
		Action:     "verify_signature",
		Operator:   c.GetString("username"),
		Result:     "OK",
		Contract:   "",
		Method:     "verifyCredential",
		SubjectDID: session.DeviceDID,
		Message:    req.RequestID,
		DetailJSON: mustJSON(gin.H{"requestId": session.RequestID, "toDomain": session.ToDomainCode}),
	})

	if c.GetString("role") == "ADMIN" {
		respData, err := h.finalizeCrossAuthApproved(&session, c.GetString("username"))
		if err != nil {
			response.InternalError(c, err.Error())
			return
		}
		respData["signatureVerifiedAt"] = now
		respData["needAdminApproval"] = false
		respData["autoApproved"] = true
		response.OK(c, respData)
		return
	}

	session.Status = "PENDING_APPROVAL"
	if err := h.DB.Save(&session).Error; err != nil {
		response.InternalError(c, "update session failed")
		return
	}
	response.OK(c, gin.H{
		"requestId":           req.RequestID,
		"status":              session.Status,
		"signatureVerifiedAt": now,
		"needAdminApproval":   true,
	})
}

type crossApproveReq struct {
	RequestID string `json:"requestId" binding:"required"`
	Approve   bool   `json:"approve"`
}

type crossRevokeReq struct {
	RequestID string `json:"requestId" binding:"required"`
	Reason    string `json:"reason"`
}

func (h *Handler) ListPendingCrossAuth(c *gin.Context) {
	if c.GetString("role") != "ADMIN" {
		response.Forbidden(c, "permission denied")
		return
	}
	var sessions []model.CrossDomainAuthSession
	if err := h.DB.Where("status IN ?", []string{"PENDING_APPROVAL"}).Order("id desc").Limit(200).Find(&sessions).Error; err != nil {
		response.InternalError(c, "query failed")
		return
	}
	response.OK(c, sessions)
}

func (h *Handler) ListMyCrossAuth(c *gin.Context) {
	role := c.GetString("role")
	domainCode := c.GetString("domainCode")
	var sessions []model.CrossDomainAuthSession

	q := h.DB.Model(&model.CrossDomainAuthSession{})
	if role != "ADMIN" {
		q = q.Where("from_domain_code = ?", domainCode)
	}
	if err := q.Order("id desc").Limit(200).Find(&sessions).Error; err != nil {
		response.InternalError(c, "query failed")
		return
	}
	response.OK(c, sessions)
}

func (h *Handler) RevokeCrossDomainAuth(c *gin.Context) {
	if c.GetString("role") != "ADMIN" {
		response.Forbidden(c, "permission denied")
		return
	}
	var req crossRevokeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	var session model.CrossDomainAuthSession
	if err := h.DB.Where("request_id = ?", strings.TrimSpace(req.RequestID)).First(&session).Error; err != nil {
		response.BadRequest(c, "request not found")
		return
	}
	if session.Status != "VERIFIED" {
		response.BadRequest(c, "only VERIFIED token can be revoked")
		return
	}
	now := time.Now()
	session.Status = "REVOKED"
	session.RevokedBy = c.GetString("username")
	session.RevokedAt = &now
	session.RevokeReason = strings.TrimSpace(req.Reason)
	if err := h.DB.Save(&session).Error; err != nil {
		response.InternalError(c, "revoke failed")
		return
	}

	// Anchor the revocation on chain so it's auditable on-chain too.
	var chainReceipt gin.H
	if anchored, err := h.AnchorOpOnChain("token_revoke", session.RequestID, session.RevokedBy+"|"+session.RevokeReason); err == nil && anchored != nil {
		chainReceipt = h.enrichChainReceipt(anchored.TxHash, anchored.BlockHeight)
	}

	auditLog := model.AuditLog{Module: "cross_auth", Action: "revoke", Operator: c.GetString("username"), Result: "OK", SubjectDID: session.DeviceDID, Message: session.RequestID, DetailJSON: mustJSON(gin.H{"reason": session.RevokeReason})}
	if chainReceipt != nil {
		if tx, ok := chainReceipt["txHash"].(string); ok {
			auditLog.TxHash = tx
		}
		if bh, ok := chainReceipt["blockHeight"].(uint64); ok {
			auditLog.BlockHeight = bh
		}
		auditLog.Contract = "anchorcc"
		auditLog.Method = "AnchorRecord"
	}
	_ = h.auditEx(auditLog)

	response.OK(c, gin.H{"requestId": session.RequestID, "status": session.Status, "revokedBy": session.RevokedBy, "revokedAt": session.RevokedAt, "reason": session.RevokeReason, "chain": chainReceipt})
}

func (h *Handler) ApproveCrossDomainAuth(c *gin.Context) {
	if c.GetString("role") != "ADMIN" {
		response.Forbidden(c, "permission denied")
		return
	}
	var req crossApproveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	var session model.CrossDomainAuthSession
	if err := h.DB.Where("request_id = ?", req.RequestID).First(&session).Error; err != nil {
		response.BadRequest(c, "request not found")
		return
	}
	if session.Status != "PENDING_APPROVAL" {
		response.BadRequest(c, "invalid status")
		return
	}

	now := time.Now()
	session.ApprovedBy = c.GetString("username")
	session.ApprovedAt = &now
	if !req.Approve {
		session.Status = "REJECTED"
		if err := h.DB.Save(&session).Error; err != nil {
			response.InternalError(c, "update failed")
			return
		}
		_ = h.auditEx(model.AuditLog{Module: "cross_auth", Action: "approve", Operator: c.GetString("username"), Result: "DENY", Method: "approve", SubjectDID: session.DeviceDID, Message: session.RequestID})
		response.OK(c, gin.H{"requestId": session.RequestID, "status": session.Status})
		return
	}

	respData, err := h.finalizeCrossAuthApproved(&session, c.GetString("username"))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, respData)
}

func (h *Handler) finalizeCrossAuthApproved(session *model.CrossDomainAuthSession, operator string) (gin.H, error) {
	okOracle, reason, detail := h.checkOracleForAuth(session.DeviceDID)
	if !okOracle {
		return nil, errors.New("oracle check denied: " + reason)
	}
	now := time.Now()
	exp := now.Add(time.Duration(session.TTLSeconds) * time.Second)
	session.Status = "VERIFIED"
	session.ApprovedBy = operator
	session.ApprovedAt = &now
	session.VerifiedAt = &now
	session.ExpiresAt = &exp

	tokenObj := gin.H{
		"subject":    session.DeviceDID,
		"srcDomain":  session.FromDomainCode,
		"dstDomain":  session.ToDomainCode,
		"resource":   session.Resource,
		"permission": session.Permission,
		"issuedAt":   now.Format(time.RFC3339),
		"expiresAt":  exp.Format(time.RFC3339),
		"requestId":  session.RequestID,
		"approvedBy": session.ApprovedBy,
	}
	session.TokenJSON = mustJSON(tokenObj)
	if err := h.DB.Save(session).Error; err != nil {
		return nil, errors.New("update session failed")
	}

	sum := sha256.Sum256([]byte(session.TokenJSON))
	digest := hex.EncodeToString(sum[:])
	anchor, err := h.Chain.Anchor("cross_auth", session.RequestID, digest)
	if err != nil {
		_ = h.auditEx(model.AuditLog{Module: "cross_auth", Action: "anchor", Operator: operator, Result: "FAIL", Contract: "anchorcc", Method: "AnchorRecord", SubjectDID: session.DeviceDID, Message: err.Error(), DetailJSON: mustJSON(gin.H{"requestId": session.RequestID, "digest": digest})})
		return nil, errors.New("fabric anchor failed")
	}
	_ = h.DB.Create(&model.ChainAnchor{BizType: "cross_auth", BizRef: session.RequestID, Digest: digest, TxHash: anchor.TxHash, BlockHeight: anchor.BlockHeight}).Error

	_ = h.auditEx(model.AuditLog{Module: "cross_auth", Action: "approve", Operator: operator, Result: "OK", Contract: "anchorcc", Method: "AnchorRecord", SubjectDID: session.DeviceDID, TxHash: anchor.TxHash, BlockHeight: anchor.BlockHeight, GasUsed: 0, Message: session.RequestID, DetailJSON: mustJSON(gin.H{"token": tokenObj, "digest": digest, "approvedBy": session.ApprovedBy})})
	return gin.H{
		"requestId":   session.RequestID,
		"status":      session.Status,
		"verifiedAt":  now,
		"expiresAt":   exp,
		"token":       tokenObj,
		"txHash":      anchor.TxHash,
		"blockHeight": anchor.BlockHeight,
		"oracle":      detail,
	}, nil
}

func (h *Handler) GetCrossAuthStatus(c *gin.Context) {
	requestID := strings.TrimSpace(c.Param("requestId"))
	if requestID == "" {
		response.BadRequest(c, "invalid requestId")
		return
	}
	var session model.CrossDomainAuthSession
	if err := h.DB.Where("request_id = ?", requestID).First(&session).Error; err != nil {
		response.BadRequest(c, "request not found")
		return
	}
	var device model.Device
	if err := h.DB.Where("device_d_id = ?", session.DeviceDID).First(&device).Error; err != nil {
		response.BadRequest(c, "device not found")
		return
	}
	if c.GetString("role") != "ADMIN" {
		if device.DomainCode != c.GetString("domainCode") {
			response.Forbidden(c, "permission denied")
			return
		}
	}
	var anchor model.ChainAnchor
	anchorOK := h.DB.Where("biz_type = ? AND biz_ref = ?", "cross_auth", session.RequestID).Order("id desc").First(&anchor).Error == nil
	resp := gin.H{
		"requestId":           session.RequestID,
		"deviceDid":           session.DeviceDID,
		"fromDomainCode":      session.FromDomainCode,
		"toDomainCode":        session.ToDomainCode,
		"resource":            session.Resource,
		"permission":          session.Permission,
		"status":              session.Status,
		"requestedBy":         session.RequestedBy,
		"signatureVerifiedAt": session.SignatureVerifiedAt,
		"approvedBy":          session.ApprovedBy,
		"approvedAt":          session.ApprovedAt,
		"revokedBy":           session.RevokedBy,
		"revokedAt":           session.RevokedAt,
		"revokeReason":        session.RevokeReason,
		"verifiedAt":          session.VerifiedAt,
		"expiresAt":           session.ExpiresAt,
	}
	if anchorOK {
		resp["txHash"] = anchor.TxHash
		resp["blockHeight"] = anchor.BlockHeight
	}
	if ok, reason, detail := h.checkOracleForAuth(session.DeviceDID); true {
		resp["oracleAllowed"] = ok
		resp["oracleReason"] = reason
		resp["oracle"] = detail
	}
	response.OK(c, resp)
}

func (h *Handler) OracleReport(c *gin.Context) {
	deviceDID := c.Param("deviceDid")
	var device model.Device
	if err := h.DB.Where("device_d_id = ?", deviceDID).First(&device).Error; err != nil {
		response.BadRequest(c, "device not found")
		return
	}
	if c.GetString("role") != "ADMIN" {
		if device.DomainCode != c.GetString("domainCode") {
			response.Forbidden(c, "device not in your domain")
			return
		}
	}

	res, err := h.runOracleAggregation(deviceDID, 8) // 8% per-flag noise so most rounds reach unanimous majority
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	_ = h.auditEx(model.AuditLog{
		Module:      "oracle",
		Action:      "report",
		Operator:    c.GetString("username"),
		Result:      "OK",
		Contract:    "anchorcc",
		Method:      "AnchorRecord",
		SubjectDID:  deviceDID,
		TxHash:      res.Anchor.TxHash,
		BlockHeight: res.Anchor.BlockHeight,
		Message:     res.Message,
		DetailJSON: mustJSON(gin.H{
			"state":       res.State,
			"aggregated":  res.Aggregated,
			"threshold":   res.Threshold,
			"submissions": res.Submissions,
			"reached":     res.Reached,
		}),
	})
	response.OK(c, gin.H{
		"state":       res.State,
		"report":      res.Aggregated,
		"txHash":      res.Anchor.TxHash,
		"blockHeight": res.Anchor.BlockHeight,
		"severity":    res.Severity,
		"message":     res.Message,
		"submissions": res.Submissions,
		"threshold":   res.Threshold,
		"participating": len(res.Submissions),
		"reachedThreshold": res.Reached,
		"aggregationId":   res.Update.ID,
	})
}

func (h *Handler) OracleCheck(c *gin.Context) {
	deviceDID := strings.TrimSpace(c.Param("deviceDid"))
	if deviceDID == "" {
		response.BadRequest(c, "invalid deviceDid")
		return
	}
	var device model.Device
	if err := h.DB.Where("device_d_id = ?", deviceDID).First(&device).Error; err != nil {
		response.BadRequest(c, "device not found")
		return
	}
	if c.GetString("role") != "ADMIN" && device.DomainCode != c.GetString("domainCode") {
		response.Forbidden(c, "permission denied")
		return
	}
	ok, reason, detail := h.checkOracleForAuth(deviceDID)
	response.OK(c, gin.H{
		"allowed": ok,
		"reason":  reason,
		"detail":  detail,
	})
}

type protectedOpReq struct {
	DeviceDID  string `json:"deviceDid" binding:"required"`
	DomainCode string `json:"domainCode" binding:"required"`
	Operation  string `json:"operation" binding:"required"`
	Payload    string `json:"payload"`
}

type protectedOperationMeta struct {
	Operation          string `json:"operation"`
	Label              string `json:"label"`
	Description        string `json:"description"`
	RequiredPermission string `json:"requiredPermission"`
	Resource           string `json:"resource"`
	PayloadTemplate    string `json:"payloadTemplate"`
}

var protectedOperationCatalog = []protectedOperationMeta{
	{
		Operation:          "READ_REMOTE_PROFILE",
		Label:              "读取设备档案",
		Description:        "读取目标域设备基础档案信息",
		RequiredPermission: "READ",
		Resource:           "profile",
		PayloadTemplate:    `{"resource":"profile"}`,
	},
	{
		Operation:          "READ_TELEMETRY",
		Label:              "读取遥测数据",
		Description:        "读取目标域设备最新遥测数据",
		RequiredPermission: "READ",
		Resource:           "telemetry",
		PayloadTemplate:    `{"resource":"telemetry","window":"5m"}`,
	},
	{
		Operation:          "WRITE_DEVICE_CONFIG",
		Label:              "下发设备配置",
		Description:        "对目标设备下发配置参数",
		RequiredPermission: "WRITE",
		Resource:           "config",
		PayloadTemplate:    `{"resource":"config","patch":{"samplingInterval":30}}`,
	},
	{
		Operation:          "RESTART_DEVICE",
		Label:              "重启设备",
		Description:        "对目标设备执行重启操作",
		RequiredPermission: "WRITE",
		Resource:           "control",
		PayloadTemplate:    `{"resource":"control","action":"restart"}`,
	},
	{
		Operation:          "ADMIN_FIRMWARE_UPGRADE",
		Label:              "固件升级",
		Description:        "执行高风险固件升级任务",
		RequiredPermission: "ADMIN",
		Resource:           "firmware",
		PayloadTemplate:    `{"resource":"firmware","version":"v1.0.1"}`,
	},
	{
		Operation:          "ADMIN_ROTATE_CREDENTIAL",
		Label:              "轮换设备密钥",
		Description:        "执行设备访问密钥轮换",
		RequiredPermission: "ADMIN",
		Resource:           "credential",
		PayloadTemplate:    `{"resource":"credential","action":"rotate"}`,
	},
}

func (h *Handler) ProtectedOperation(c *gin.Context) {
	var req protectedOpReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}

	// Cross-domain semantics:
	//   srcDID = source device that holds the AuthToken (session.deviceDID)
	//   opDID  = device actually being operated on (may belong to target domain)
	// Middleware sets both; fall back to req.DeviceDID if missing.
	opDID := c.GetString("crossOpDeviceDid")
	if opDID == "" {
		opDID = req.DeviceDID
	}
	srcDID := c.GetString("crossSrcDeviceDid")
	if srcDID == "" {
		srcDID = req.DeviceDID
	}

	var device model.Device
	if err := h.DB.Where("device_d_id = ?", opDID).First(&device).Error; err != nil {
		response.BadRequest(c, "device not found")
		return
	}
	// Operator must either be ADMIN or own the SOURCE side of the
	// cross-domain session — they don't need to "own" the target device.
	if c.GetString("role") != "ADMIN" {
		var src model.Device
		if err := h.DB.Where("device_d_id = ?", srcDID).First(&src).Error; err != nil || src.DomainCode != c.GetString("domainCode") {
			response.Forbidden(c, "source device not in your domain")
			return
		}
	}
	targetDomain := strings.TrimSpace(middleware.HeaderDecoded(c, "X-Target-Domain"))
	if targetDomain == "" || targetDomain != strings.TrimSpace(req.DomainCode) {
		response.BadRequest(c, "domainCode does not match X-Target-Domain")
		return
	}
	if device.RuntimeState != "TRUSTED" {
		response.Forbidden(c, "device runtime state is not trusted")
		return
	}
	session, reason := h.latestUsableVerifiedSession(srcDID, targetDomain)
	if session == nil {
		response.Forbidden(c, reason)
		return
	}
	meta, ok := findProtectedOperationMeta(req.Operation)
	if !ok {
		response.BadRequest(c, "unknown operation")
		return
	}
	if !permissionAllows(session.Permission, meta.RequiredPermission) {
		response.Forbidden(c, "operation permission denied by cross-domain token")
		return
	}
	if !resourceAllowed(session.Resource, meta.Resource) {
		response.Forbidden(c, "operation resource denied by cross-domain token")
		return
	}

	op := model.ProtectedOperation{
		DeviceDID:  opDID,
		DomainCode: req.DomainCode,
		Operation:  meta.Operation,
		Payload:    req.Payload,
		CreatedBy:  c.GetString("username"),
	}
	if err := h.DB.Create(&op).Error; err != nil {
		response.InternalError(c, "create operation failed")
		return
	}

	// Real-ize: mutate device state for write ops; read ops return no-mutation.
	effect, effectErr := h.applyOperationEffect(meta.Operation, &device, req.Payload)
	effectMsg := ""
	if effect != nil {
		effectMsg = effect.Description
	}
	if effectErr != nil {
		effectMsg = "effect_error: " + effectErr.Error()
	}

	// Anchor write operations on chain (skip pure-read ops).
	var chainReceipt gin.H
	isWrite := meta.RequiredPermission == "WRITE" || meta.RequiredPermission == "ADMIN"
	if isWrite && effectErr == nil {
		anchored, err := h.AnchorOpOnChain("protected_op", opDID, req.Operation+"|"+req.Payload)
		if err == nil && anchored != nil {
			chainReceipt = h.enrichChainReceipt(anchored.TxHash, anchored.BlockHeight)
		} else if err != nil {
			chainReceipt = gin.H{"error": err.Error()}
		}
	}

	auditLog := model.AuditLog{
		Module:     "operation",
		Action:     "protected_execute",
		Operator:   c.GetString("username"),
		Result:     "OK",
		Contract:   "",
		Method:     meta.Operation,
		SubjectDID: opDID,
		Message:    meta.Operation,
		DetailJSON: mustJSON(gin.H{"payload": req.Payload, "domain": req.DomainCode, "srcDeviceDid": srcDID, "requiredPermission": meta.RequiredPermission, "resource": meta.Resource, "effect": effectMsg}),
	}
	if chainReceipt != nil {
		if tx, ok := chainReceipt["txHash"].(string); ok {
			auditLog.TxHash = tx
		}
		if bh, ok := chainReceipt["blockHeight"].(uint64); ok {
			auditLog.BlockHeight = bh
		}
		auditLog.Contract = "anchorcc"
	}
	_ = h.auditEx(auditLog)

	response.OK(c, gin.H{
		"id":                 op.ID,
		"deviceDid":          op.DeviceDID,
		"domainCode":         op.DomainCode,
		"operation":          op.Operation,
		"payload":            op.Payload,
		"createdBy":          op.CreatedBy,
		"createdAt":          op.CreatedAt,
		"requiredPermission": meta.RequiredPermission,
		"resource":           meta.Resource,
		"effect":             effect,
		"chain":              chainReceipt,
	})
}

func (h *Handler) ListOperationCatalog(c *gin.Context) {
	deviceDID := strings.TrimSpace(c.Query("deviceDid"))
	targetDomain := strings.TrimSpace(c.Query("targetDomain"))
	if deviceDID == "" || targetDomain == "" {
		response.BadRequest(c, "deviceDid and targetDomain required")
		return
	}
	var device model.Device
	if err := h.DB.Where("device_d_id = ?", deviceDID).First(&device).Error; err != nil {
		response.BadRequest(c, "device not found")
		return
	}
	if c.GetString("role") != "ADMIN" && device.DomainCode != c.GetString("domainCode") {
		response.Forbidden(c, "permission denied")
		return
	}
	session, reason := h.latestUsableVerifiedSession(deviceDID, targetDomain)
	if session == nil {
		response.Forbidden(c, reason)
		return
	}
	items := make([]protectedOperationMeta, 0)
	for _, m := range protectedOperationCatalog {
		if !permissionAllows(session.Permission, m.RequiredPermission) {
			continue
		}
		if !resourceAllowed(session.Resource, m.Resource) {
			continue
		}
		items = append(items, m)
	}
	response.OK(c, gin.H{
		"sessionPermission": strings.ToUpper(strings.TrimSpace(session.Permission)),
		"sessionResource":   strings.TrimSpace(session.Resource),
		"expiresAt":         session.ExpiresAt,
		"items":             items,
	})
}

func (h *Handler) ListOperationHistory(c *gin.Context) {
	deviceDID := strings.TrimSpace(c.Query("deviceDid"))
	targetDomain := strings.TrimSpace(c.Query("targetDomain"))
	if deviceDID == "" {
		response.BadRequest(c, "deviceDid required")
		return
	}
	var device model.Device
	if err := h.DB.Where("device_d_id = ?", deviceDID).First(&device).Error; err != nil {
		response.BadRequest(c, "device not found")
		return
	}
	if c.GetString("role") != "ADMIN" && device.DomainCode != c.GetString("domainCode") {
		response.Forbidden(c, "permission denied")
		return
	}
	limit := 20
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	q := h.DB.Model(&model.ProtectedOperation{}).Where("device_d_id = ?", deviceDID)
	if targetDomain != "" {
		q = q.Where("domain_code = ?", targetDomain)
	}
	var ops []model.ProtectedOperation
	if err := q.Order("id desc").Limit(limit).Find(&ops).Error; err != nil {
		response.InternalError(c, "query operation history failed")
		return
	}
	response.OK(c, ops)
}

func (h *Handler) DashboardOverview(c *gin.Context) {
	var deviceTotal int64
	_ = h.DB.Model(&model.Device{}).Count(&deviceTotal).Error

	startOfDay := time.Now().Truncate(24 * time.Hour)
	var crossToday int64
	_ = h.DB.Model(&model.CrossDomainAuthSession{}).
		Where("status = ? AND verified_at >= ?", "VERIFIED", startOfDay).
		Count(&crossToday).Error
	var alertsToday int64
	_ = h.DB.Model(&model.DeviceStateUpdate{}).
		Where("severity > 0 AND created_at >= ?", startOfDay).
		Count(&alertsToday).Error

	onlineCount, lastOracleAt := h.calcOnlineCountAndLastOracle(15 * time.Minute)

	var recentSessions []model.CrossDomainAuthSession
	_ = h.DB.Order("id desc").Limit(10).Find(&recentSessions).Error

	refs := make([]string, 0, len(recentSessions))
	for _, s := range recentSessions {
		refs = append(refs, s.RequestID)
	}
	anchorMap := map[string]model.ChainAnchor{}
	if len(refs) > 0 {
		var anchors []model.ChainAnchor
		_ = h.DB.Where("biz_type = ? AND biz_ref IN ?", "cross_auth", refs).Find(&anchors).Error
		for _, a := range anchors {
			anchorMap[a.BizRef] = a
		}
	}

	recent := make([]gin.H, 0, len(recentSessions))
	for _, s := range recentSessions {
		a, ok := anchorMap[s.RequestID]
		item := gin.H{
			"requestId":  s.RequestID,
			"deviceDid":  s.DeviceDID,
			"fromDomain": s.FromDomainCode,
			"toDomain":   s.ToDomainCode,
			"status":     s.Status,
			"verifiedAt": s.VerifiedAt,
			"expiresAt":  s.ExpiresAt,
		}
		if ok {
			item["txHash"] = a.TxHash
			item["blockHeight"] = a.BlockHeight
		}
		recent = append(recent, item)
	}

	oracleStatus := gin.H{
		"nodes":        gin.H{"total": 5, "threshold": 3},
		"successRate":  0.994,
		"lastCommitAt": lastOracleAt,
	}

	response.OK(c, gin.H{
		"kpi": gin.H{
			"devicesTotal":   deviceTotal,
			"devicesOnline":  onlineCount,
			"crossAuthToday": crossToday,
			"alertsToday":    alertsToday,
		},
		"recentCrossAuth": recent,
		"oracle":          oracleStatus,
	})
}

func (h *Handler) MonitorOverview(c *gin.Context) {
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if hours <= 0 || hours > 168 {
		hours = 24
	}
	from := time.Now().Add(-time.Duration(hours) * time.Hour)

	type row struct {
		Bucket    string `gorm:"column:bucket"`
		OnlineCnt int64  `gorm:"column:onlineCnt"`
		AlertCnt  int64  `gorm:"column:alertCnt"`
		MarkerCnt int64  `gorm:"column:markerCnt"`
	}
	var rows []row
	_ = h.DB.Raw(
		"SELECT DATE_FORMAT(created_at,'%Y-%m-%d %H:00:00') AS bucket, "+
			"COUNT(DISTINCT CASE WHEN online = TRUE THEN device_d_id END) AS onlineCnt, "+
			"SUM(CASE WHEN severity > 0 THEN 1 ELSE 0 END) AS alertCnt, "+
			"SUM(CASE WHEN severity > 0 THEN 1 ELSE 0 END) AS markerCnt "+
			"FROM device_state_updates WHERE created_at >= ? GROUP BY bucket ORDER BY bucket",
		from,
	).Scan(&rows).Error

	series := make([]gin.H, 0, len(rows))
	markers := make([]string, 0, len(rows))
	for _, r := range rows {
		series = append(series, gin.H{"bucket": r.Bucket, "online": r.OnlineCnt, "alerts": r.AlertCnt})
		if r.MarkerCnt > 0 {
			markers = append(markers, r.Bucket)
		}
	}

	response.OK(c, gin.H{
		"series":  series,
		"markers": markers,
	})
}

func (h *Handler) MonitorDevices(c *gin.Context) {
	var devices []model.Device
	query := h.DB.Order("id desc")
	if c.GetString("role") != "ADMIN" {
		query = query.Where("domain_code = ?", c.GetString("domainCode"))
	}
	_ = query.Find(&devices).Error

	var updates []model.DeviceStateUpdate
	_ = h.DB.Raw(
		"SELECT s1.* FROM device_state_updates s1 " +
			"JOIN (SELECT device_d_id, MAX(created_at) mx FROM device_state_updates GROUP BY device_d_id) s2 " +
			"ON s1.device_d_id = s2.device_d_id AND s1.created_at = s2.mx",
	).Scan(&updates).Error

	lastMap := map[string]model.DeviceStateUpdate{}
	for _, u := range updates {
		lastMap[u.DeviceDID] = u
	}

	rows := make([]gin.H, 0, len(devices))
	for _, d := range devices {
		row := gin.H{
			"id":           d.ID,
			"deviceDid":    d.DeviceDID,
			"domainCode":   d.DomainCode,
			"displayName":  d.DisplayName,
			"deviceType":   d.DeviceType,
			"runtimeState": d.RuntimeState,
			"createdAt":    d.CreatedAt,
			"updatedAt":    d.UpdatedAt,
		}
		if u, ok := lastMap[d.DeviceDID]; ok {
			row["lastReport"] = u
		}
		rows = append(rows, row)
	}
	response.OK(c, rows)
}

func (h *Handler) MonitorAlerts(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var alerts []model.DeviceStateUpdate
	query := h.DB.Where("severity > 0")
	if c.GetString("role") != "ADMIN" {
		var dids []string
		_ = h.DB.Model(&model.Device{}).Where("domain_code = ?", c.GetString("domainCode")).Pluck("device_d_id", &dids).Error
		if len(dids) == 0 {
			response.OK(c, []model.DeviceStateUpdate{})
			return
		}
		query = query.Where("device_d_id IN ?", dids)
	}
	if err := query.Order("id desc").Limit(limit).Find(&alerts).Error; err != nil {
		response.InternalError(c, "query failed")
		return
	}
	response.OK(c, alerts)
}

func (h *Handler) AuditLogs(c *gin.Context) {
	var logs []model.AuditLog
	if err := h.DB.Order("id desc").Limit(200).Find(&logs).Error; err != nil {
		response.InternalError(c, "query failed")
		return
	}
	response.OK(c, logs)
}

func (h *Handler) AuditSearch(c *gin.Context) {
	module := strings.TrimSpace(c.Query("module"))
	result := strings.TrimSpace(c.Query("result"))
	q := strings.TrimSpace(c.Query("q"))
	fromStr := strings.TrimSpace(c.Query("from"))
	toStr := strings.TrimSpace(c.Query("to"))

	query := h.DB.Model(&model.AuditLog{})
	if c.GetString("role") != "ADMIN" {
		query = query.Where("operator = ?", c.GetString("username"))
	}
	if module != "" {
		query = query.Where("module = ?", module)
	}
	if result != "" {
		query = query.Where("result = ?", result)
	}
	if q != "" {
		like := "%" + q + "%"
		query = query.Where("message LIKE ? OR subject_did LIKE ? OR tx_hash LIKE ? OR action LIKE ? OR method LIKE ?", like, like, like, like, like)
	}
	if fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			query = query.Where("occurred_at >= ?", t)
		}
	}
	if toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			query = query.Where("occurred_at <= ?", t)
		}
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	if limit <= 0 || limit > 500 {
		limit = 200
	}

	var logs []model.AuditLog
	if err := query.Order("id desc").Limit(limit).Find(&logs).Error; err != nil {
		response.InternalError(c, "query failed")
		return
	}
	response.OK(c, logs)
}

// AuditExport streams the same filtered query as AuditSearch, but as a CSV
// download. UTF-8 BOM is written first so Excel renders Chinese correctly.
// Honors the same five query params (module/result/q/from/to) and the same
// RBAC rule (non-ADMIN sees only their own rows). Caps at 5000 rows.
func (h *Handler) AuditExport(c *gin.Context) {
	module := strings.TrimSpace(c.Query("module"))
	result := strings.TrimSpace(c.Query("result"))
	q := strings.TrimSpace(c.Query("q"))
	fromStr := strings.TrimSpace(c.Query("from"))
	toStr := strings.TrimSpace(c.Query("to"))

	query := h.DB.Model(&model.AuditLog{})
	if c.GetString("role") != "ADMIN" {
		query = query.Where("operator = ?", c.GetString("username"))
	}
	if module != "" {
		query = query.Where("module = ?", module)
	}
	if result != "" {
		query = query.Where("result = ?", result)
	}
	if q != "" {
		like := "%" + q + "%"
		query = query.Where("message LIKE ? OR subject_did LIKE ? OR tx_hash LIKE ? OR action LIKE ? OR method LIKE ?", like, like, like, like, like)
	}
	if fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			query = query.Where("occurred_at >= ?", t)
		}
	}
	if toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			query = query.Where("occurred_at <= ?", t)
		}
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "5000"))
	if limit <= 0 || limit > 50000 {
		limit = 5000
	}

	var logs []model.AuditLog
	if err := query.Order("id desc").Limit(limit).Find(&logs).Error; err != nil {
		response.InternalError(c, "query failed")
		return
	}

	filename := "audit_log_" + time.Now().Format("20060102_150405") + ".csv"
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Header("Cache-Control", "no-store")

	w := c.Writer
	// UTF-8 BOM so Excel reads the CSV as UTF-8 (otherwise Chinese is garbled).
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	cw := csv.NewWriter(w)
	defer cw.Flush()
	_ = cw.Write([]string{
		"时间", "模块", "动作", "操作人", "结果",
		"合约", "调用方法", "From", "主体DID",
		"交易哈希", "区块高度", "Gas", "消息", "详情JSON",
	})
	for _, r := range logs {
		_ = cw.Write([]string{
			r.OccurredAt.Format("2006-01-02 15:04:05"),
			r.Module,
			r.Action,
			r.Operator,
			r.Result,
			r.Contract,
			r.Method,
			r.From,
			r.SubjectDID,
			r.TxHash,
			strconv.FormatUint(r.BlockHeight, 10),
			strconv.FormatUint(r.GasUsed, 10),
			r.Message,
			r.DetailJSON,
		})
	}
	cw.Flush()

	_ = h.auditEx(model.AuditLog{
		Module:   "audit",
		Action:   "export_csv",
		Operator: c.GetString("username"),
		Result:   "OK",
		Message:  "exported " + strconv.Itoa(len(logs)) + " rows",
		DetailJSON: mustJSON(gin.H{
			"module": module, "result": result, "q": q, "from": fromStr, "to": toStr, "rows": len(logs),
		}),
	})
}

type trustPolicyReq struct {
	FromDomainCode string `json:"fromDomainCode" binding:"required"`
	ToDomainCode   string `json:"toDomainCode" binding:"required"`
	PolicyLevel    string `json:"policyLevel"`
	AllowedPerms   string `json:"allowedPerms"`
	AllowedRes     string `json:"allowedRes"`
	Enabled        bool   `json:"enabled"`
}

func (h *Handler) ListTrustPolicies(c *gin.Context) {
	role := c.GetString("role")
	domain := c.GetString("domainCode")
	var rows []model.DomainTrustPolicy
	q := h.DB.Order("id desc")
	if role != "ADMIN" {
		q = q.Where("from_domain_code = ?", domain)
	}
	if err := q.Find(&rows).Error; err != nil {
		response.InternalError(c, "query failed")
		return
	}
	response.OK(c, rows)
}

func (h *Handler) UpsertTrustPolicy(c *gin.Context) {
	role := c.GetString("role")
	var req trustPolicyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	from := strings.TrimSpace(req.FromDomainCode)
	to := strings.TrimSpace(req.ToDomainCode)
	if from == "" || to == "" {
		response.BadRequest(c, "from/to domain required")
		return
	}
	if role != "ADMIN" && from != c.GetString("domainCode") {
		response.Forbidden(c, "domain admin can only edit own policies")
		return
	}
	level := strings.ToUpper(strings.TrimSpace(req.PolicyLevel))
	if level == "" {
		level = "ALLOW"
	}
	if level != "ALLOW" && level != "DENY" {
		response.BadRequest(c, "invalid policyLevel")
		return
	}
	perms := strings.ToUpper(strings.TrimSpace(req.AllowedPerms))
	if perms == "" {
		perms = "READ"
	}
	res := strings.TrimSpace(req.AllowedRes)
	if res == "" {
		res = "*"
	}
	var policy model.DomainTrustPolicy
	err := h.DB.Where("from_domain_code = ? AND to_domain_code = ?", from, to).First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		policy = model.DomainTrustPolicy{
			FromDomainCode: from,
			ToDomainCode:   to,
			PolicyLevel:    level,
			AllowedPerms:   perms,
			AllowedRes:     res,
			Enabled:        req.Enabled,
			UpdatedBy:      c.GetString("username"),
		}
		if err := h.DB.Create(&policy).Error; err != nil {
			response.InternalError(c, "save failed")
			return
		}
	} else if err == nil {
		policy.PolicyLevel = level
		policy.AllowedPerms = perms
		policy.AllowedRes = res
		policy.Enabled = req.Enabled
		policy.UpdatedBy = c.GetString("username")
		if err := h.DB.Save(&policy).Error; err != nil {
			response.InternalError(c, "save failed")
			return
		}
	} else {
		response.InternalError(c, "query failed")
		return
	}
	_ = h.auditEx(model.AuditLog{Module: "policy", Action: "upsert", Operator: c.GetString("username"), Result: "OK", Message: from + "->" + to, DetailJSON: mustJSON(policy)})
	response.OK(c, policy)
}

func (h *Handler) DeleteTrustPolicy(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.BadRequest(c, "invalid id")
		return
	}
	role := c.GetString("role")
	var policy model.DomainTrustPolicy
	if err := h.DB.Where("id = ?", id).First(&policy).Error; err != nil {
		response.BadRequest(c, "policy not found")
		return
	}
	if role != "ADMIN" && policy.FromDomainCode != c.GetString("domainCode") {
		response.Forbidden(c, "permission denied")
		return
	}
	if err := h.DB.Delete(&policy).Error; err != nil {
		response.InternalError(c, "delete failed")
		return
	}
	_ = h.auditEx(model.AuditLog{Module: "policy", Action: "delete", Operator: c.GetString("username"), Result: "OK", Message: policy.FromDomainCode + "->" + policy.ToDomainCode})
	response.OK(c, gin.H{"id": policy.ID, "deleted": true})
}

type oracleNodeReq struct {
	NodeName  string `json:"nodeName" binding:"required"`
	PublicKey string `json:"publicKey" binding:"required"`
}

func (h *Handler) ListOracleNodes(c *gin.Context) {
	if c.GetString("role") != "ADMIN" {
		response.Forbidden(c, "permission denied")
		return
	}
	var nodes []model.OracleNode
	if err := h.DB.Order("id desc").Find(&nodes).Error; err != nil {
		response.InternalError(c, "query failed")
		return
	}
	response.OK(c, nodes)
}

func (h *Handler) RegisterOracleNode(c *gin.Context) {
	if c.GetString("role") != "ADMIN" {
		response.Forbidden(c, "permission denied")
		return
	}
	var req oracleNodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	pubKey, _, err := validateOracleNodePublicKey(req.PublicKey)
	if err != nil {
		response.BadRequest(c, "invalid oracle public key: "+err.Error())
		return
	}
	node := model.OracleNode{
		NodeName:  strings.TrimSpace(req.NodeName),
		PublicKey: pubKey,
		Status:    "ACTIVE",
		LastSeen:  time.Now(),
	}
	if err := h.DB.Create(&node).Error; err != nil {
		response.BadRequest(c, "node exists or invalid")
		return
	}
	_ = h.auditEx(model.AuditLog{Module: "system", Action: "oracle_node_register", Operator: c.GetString("username"), Result: "OK", Message: node.NodeName})
	response.OK(c, node)
}

type rotateOracleKeyReq struct {
	PublicKey string `json:"publicKey" binding:"required"`
}

func (h *Handler) RotateOracleNodeKey(c *gin.Context) {
	if c.GetString("role") != "ADMIN" {
		response.Forbidden(c, "permission denied")
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		response.BadRequest(c, "invalid node id")
		return
	}
	var req rotateOracleKeyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	pubKey, _, err := validateOracleNodePublicKey(req.PublicKey)
	if err != nil {
		response.BadRequest(c, "invalid oracle public key: "+err.Error())
		return
	}
	var node model.OracleNode
	if err := h.DB.Where("id = ?", id).First(&node).Error; err != nil {
		response.BadRequest(c, "node not found")
		return
	}
	node.PublicKey = pubKey
	node.UpdatedAt = time.Now()
	node.LastSeen = time.Now()
	if err := h.DB.Save(&node).Error; err != nil {
		response.InternalError(c, "rotate failed")
		return
	}
	_ = h.auditEx(model.AuditLog{Module: "system", Action: "oracle_node_rotate_key", Operator: c.GetString("username"), Result: "OK", Message: node.NodeName})
	response.OK(c, node)
}

type thresholdReq struct {
	Threshold int `json:"threshold"`
}

func (h *Handler) GetOracleThreshold(c *gin.Context) {
	if c.GetString("role") != "ADMIN" {
		response.Forbidden(c, "permission denied")
		return
	}
	var setting model.SystemSetting
	if err := h.DB.Where("conf_key = ?", "oracle_threshold").First(&setting).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.OK(c, gin.H{"threshold": 3})
			return
		}
		response.InternalError(c, "query failed")
		return
	}
	val, _ := strconv.Atoi(setting.ConfValue)
	if val <= 0 {
		val = 3
	}
	response.OK(c, gin.H{"threshold": val, "updatedBy": setting.UpdatedBy, "updatedAt": setting.UpdatedAt})
}

func (h *Handler) SetOracleThreshold(c *gin.Context) {
	if c.GetString("role") != "ADMIN" {
		response.Forbidden(c, "permission denied")
		return
	}
	var req thresholdReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	if req.Threshold <= 0 || req.Threshold > 100 {
		response.BadRequest(c, "invalid threshold")
		return
	}
	var setting model.SystemSetting
	err := h.DB.Where("conf_key = ?", "oracle_threshold").First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		setting = model.SystemSetting{
			ConfKey:   "oracle_threshold",
			ConfValue: strconv.Itoa(req.Threshold),
			UpdatedBy: c.GetString("username"),
		}
		if err := h.DB.Create(&setting).Error; err != nil {
			response.InternalError(c, "save failed")
			return
		}
	} else if err == nil {
		setting.ConfValue = strconv.Itoa(req.Threshold)
		setting.UpdatedBy = c.GetString("username")
		if err := h.DB.Save(&setting).Error; err != nil {
			response.InternalError(c, "save failed")
			return
		}
	} else {
		response.InternalError(c, "query failed")
		return
	}
	_ = h.auditEx(model.AuditLog{Module: "system", Action: "oracle_threshold_set", Operator: c.GetString("username"), Result: "OK", Message: setting.ConfValue})
	response.OK(c, gin.H{"threshold": req.Threshold})
}

func (h *Handler) calcOnlineCountAndLastOracle(window time.Duration) (int64, *time.Time) {
	var updates []model.DeviceStateUpdate
	_ = h.DB.Raw(
		"SELECT s1.* FROM device_state_updates s1 " +
			"JOIN (SELECT device_d_id, MAX(created_at) mx FROM device_state_updates GROUP BY device_d_id) s2 " +
			"ON s1.device_d_id = s2.device_d_id AND s1.created_at = s2.mx",
	).Scan(&updates).Error

	cutoff := time.Now().Add(-window)
	online := int64(0)
	var last *time.Time
	for _, u := range updates {
		uCreated := u.CreatedAt
		if last == nil || uCreated.After(*last) {
			v := uCreated
			last = &v
		}
		if u.Online && uCreated.After(cutoff) {
			online++
		}
	}
	return online, last
}

func (h *Handler) auditEx(a model.AuditLog) error {
	if strings.TrimSpace(a.Operator) == "" {
		a.Operator = "system"
	}
	if a.Result == "" {
		a.Result = "OK"
	}
	if err := h.DB.Create(&a).Error; err != nil && !errors.Is(err, gorm.ErrInvalidDB) {
		return err
	}
	return nil
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func isValidDeviceType(v string) bool {
	switch strings.TrimSpace(v) {
	case "传感器", "执行器", "网关", "摄像头", "门禁设备", "环境监测设备", "工业控制器", "智能电表", "智能家电", "车载终端", "其他":
		return true
	default:
		return false
	}
}

func splitUpperList(v string) map[string]bool {
	out := map[string]bool{}
	parts := strings.Split(v, ",")
	for _, p := range parts {
		item := strings.ToUpper(strings.TrimSpace(p))
		if item != "" {
			out[item] = true
		}
	}
	return out
}

func splitRawList(v string) map[string]bool {
	out := map[string]bool{}
	parts := strings.Split(v, ",")
	for _, p := range parts {
		item := strings.TrimSpace(p)
		if item != "" {
			out[item] = true
		}
	}
	return out
}

func validateOracleNodePublicKey(raw string) (string, string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", errors.New("empty public key")
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\\n", "\n")
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return "", "", errors.New("invalid PEM format")
	}
	if block.Type != "PUBLIC KEY" {
		return "", "", errors.New("PEM type must be PUBLIC KEY")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", "", errors.New("invalid PKIX public key")
	}
	switch pub := pub.(type) {
	case *rsa.PublicKey:
		if pub.N == nil || pub.E <= 1 {
			return "", "", errors.New("invalid RSA key")
		}
		return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: block.Bytes})), "RSA", nil
	case *ecdsa.PublicKey:
		if pub.X == nil || pub.Y == nil || pub.Curve == nil {
			return "", "", errors.New("invalid ECDSA key")
		}
		return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: block.Bytes})), "ECDSA", nil
	case ed25519.PublicKey:
		if len(pub) == 0 {
			return "", "", errors.New("invalid ED25519 key")
		}
		return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: block.Bytes})), "ED25519", nil
	default:
		return "", "", errors.New("unsupported key algorithm")
	}
}

func findProtectedOperationMeta(operation string) (protectedOperationMeta, bool) {
	op := strings.ToUpper(strings.TrimSpace(operation))
	for _, m := range protectedOperationCatalog {
		if m.Operation == op {
			return m, true
		}
	}
	return protectedOperationMeta{}, false
}

func permissionRank(permission string) int {
	switch strings.ToUpper(strings.TrimSpace(permission)) {
	case "ADMIN":
		return 3
	case "WRITE":
		return 2
	case "READ":
		return 1
	default:
		return 0
	}
}

func permissionAllows(granted, required string) bool {
	return permissionRank(granted) >= permissionRank(required)
}

func resourceAllowed(granted, required string) bool {
	granted = strings.TrimSpace(granted)
	required = strings.TrimSpace(required)
	if required == "" {
		return true
	}
	if granted == "" {
		return false
	}
	allowList := splitRawList(granted)
	if allowList["*"] {
		return true
	}
	return allowList[required]
}

func (h *Handler) checkTrustPolicy(fromDomain, toDomain, permission, resource string) (bool, string) {
	if strings.TrimSpace(fromDomain) == strings.TrimSpace(toDomain) {
		return true, "same-domain"
	}
	var policy model.DomainTrustPolicy
	if err := h.DB.Where("from_domain_code = ? AND to_domain_code = ? AND enabled = ?", fromDomain, toDomain, true).First(&policy).Error; err != nil {
		return false, "policy not found"
	}
	if strings.ToUpper(policy.PolicyLevel) == "DENY" {
		return false, "policy level deny"
	}
	perms := splitUpperList(policy.AllowedPerms)
	if len(perms) > 0 {
		p := strings.ToUpper(strings.TrimSpace(permission))
		if p == "" || !perms[p] {
			return false, "permission not allowed"
		}
	}
	resList := splitRawList(policy.AllowedRes)
	if len(resList) > 0 && !resList["*"] {
		r := strings.TrimSpace(resource)
		if r == "" || !resList[r] {
			return false, "resource not allowed"
		}
	}
	return true, "allowed"
}

func (h *Handler) latestUsableVerifiedSession(deviceDID, targetDomain string) (*model.CrossDomainAuthSession, string) {
	var session model.CrossDomainAuthSession
	err := h.DB.Where("device_d_id = ? AND to_domain_code = ? AND status = ?", deviceDID, targetDomain, "VERIFIED").
		Order("verified_at desc").
		First(&session).Error
	if err != nil {
		return nil, "cross-domain authentication required"
	}
	now := time.Now()
	if session.ExpiresAt != nil {
		if now.After(*session.ExpiresAt) {
			return nil, "cross-domain authentication expired"
		}
	} else if session.VerifiedAt == nil || time.Since(*session.VerifiedAt) > 30*time.Minute {
		return nil, "cross-domain authentication expired"
	}
	return &session, ""
}

func (h *Handler) getOracleThreshold() int {
	var setting model.SystemSetting
	if err := h.DB.Where("conf_key = ?", "oracle_threshold").First(&setting).Error; err != nil {
		return 3
	}
	v, err := strconv.Atoi(strings.TrimSpace(setting.ConfValue))
	if err != nil || v <= 0 {
		return 3
	}
	return v
}

func (h *Handler) activeOracleNodes() int64 {
	var cnt int64
	_ = h.DB.Model(&model.OracleNode{}).Where("status = ?", "ACTIVE").Count(&cnt).Error
	return cnt
}

func (h *Handler) latestOracleState(deviceDID string) (*model.DeviceStateUpdate, error) {
	var s model.DeviceStateUpdate
	if err := h.DB.Where("device_d_id = ?", deviceDID).Order("id desc").First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (h *Handler) checkOracleForAuth(deviceDID string) (bool, string, gin.H) {
	threshold := h.getOracleThreshold()
	activeNodes := h.activeOracleNodes()
	state, err := h.latestOracleState(deviceDID)
	if err != nil {
		return false, "missing oracle state", gin.H{
			"threshold":   threshold,
			"activeNodes": activeNodes,
			"state":       nil,
		}
	}
	ageSec := int(time.Since(state.CreatedAt).Seconds())
	allowed := true
	reason := "ok"
	if activeNodes < int64(threshold) {
		allowed = false
		reason = "active oracle nodes below threshold"
	} else if ageSec > 600 {
		allowed = false
		reason = "oracle state expired"
	} else if strings.ToUpper(strings.TrimSpace(state.StateLabel)) != "TRUSTED" || state.Score < 70 {
		allowed = false
		reason = "oracle state not trusted"
	}
	return allowed, reason, gin.H{
		"threshold":    threshold,
		"activeNodes":  activeNodes,
		"stateLabel":   state.StateLabel,
		"score":        state.Score,
		"severity":     state.Severity,
		"message":      state.Message,
		"lastReportAt": state.CreatedAt,
		"ageSec":       ageSec,
	}
}

func deviceTypeToDIDToken(v string) string {
	switch strings.TrimSpace(v) {
	case "传感器":
		return "sensor"
	case "执行器":
		return "actuator"
	case "网关":
		return "gateway"
	case "摄像头":
		return "camera"
	case "门禁设备":
		return "access-control"
	case "环境监测设备":
		return "env-monitor"
	case "工业控制器":
		return "plc"
	case "智能电表":
		return "smart-meter"
	case "智能家电":
		return "smart-appliance"
	case "车载终端":
		return "vehicle-terminal"
	case "其他":
		return "other"
	default:
		return "device"
	}
}

func isValidDeviceDID(deviceDID string, domain string, deviceType string) bool {
	deviceDID = strings.TrimSpace(deviceDID)
	domain = strings.ToLower(strings.TrimSpace(domain))
	typeToken := deviceTypeToDIDToken(deviceType)
	pattern := fmt.Sprintf(`^did:iot:%s:%s:[a-z0-9-]+$`, regexp.QuoteMeta(domain), regexp.QuoteMeta(typeToken))
	ok, err := regexp.MatchString(pattern, strings.ToLower(deviceDID))
	return err == nil && ok
}

func (h *Handler) audit(module string, action string, operator string, result string, msg string) error {
	if operator == "" {
		operator = "system"
	}
	record := model.AuditLog{
		Module:   module,
		Action:   action,
		Operator: operator,
		Result:   result,
		Message:  msg,
	}
	err := h.DB.Create(&record).Error
	if err != nil && !errors.Is(err, gorm.ErrInvalidDB) {
		return err
	}
	return nil
}
