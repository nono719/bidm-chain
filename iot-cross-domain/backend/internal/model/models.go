package model

import "time"

type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"size:64;uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"`
	DisplayName  string    `gorm:"size:128;not null;default:''" json:"displayName"`
	Role         string    `gorm:"size:32;not null" json:"role"`
	DomainCode   string    `gorm:"size:64;index;not null;default:''" json:"domainCode"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Domain struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Code      string    `gorm:"size:64;uniqueIndex;not null" json:"code"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

type Device struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	DeviceDID    string    `gorm:"size:128;uniqueIndex;not null" json:"deviceDid"`
	DomainCode   string    `gorm:"size:64;index;not null" json:"domainCode"`
	DisplayName  string    `gorm:"size:128;not null" json:"displayName"`
	Credential   string    `gorm:"size:255;not null" json:"credential"`
	DeviceType   string    `gorm:"size:64;not null;default:'UNKNOWN'" json:"deviceType"`
	PublicKeyJWK string    `gorm:"type:text" json:"publicKeyJwk"`
	MetadataJSON string    `gorm:"type:text" json:"metadataJson"`
	Lifecycle    string    `gorm:"size:32;index;not null;default:'ACTIVE'" json:"lifecycle"`
	RuntimeState string    `gorm:"size:64;not null;default:'UNKNOWN'" json:"runtimeState"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type CrossDomainAuthSession struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	RequestID           string     `gorm:"size:64;uniqueIndex;not null" json:"requestId"`
	DeviceDID           string     `gorm:"size:128;index;not null" json:"deviceDid"`
	FromDomainCode      string     `gorm:"size:64;not null" json:"fromDomainCode"`
	ToDomainCode        string     `gorm:"size:64;not null" json:"toDomainCode"`
	Resource            string     `gorm:"size:255;not null;default:''" json:"resource"`
	Permission          string     `gorm:"size:64;not null;default:''" json:"permission"`
	TTLSeconds          int        `gorm:"not null;default:1800" json:"ttlSeconds"`
	Challenge           string     `gorm:"size:128;not null" json:"challenge"`
	Status              string     `gorm:"size:32;index;not null" json:"status"`
	RequestedBy         string     `gorm:"size:64;not null;default:''" json:"requestedBy"`
	SignatureVerifiedAt *time.Time `json:"signatureVerifiedAt"`
	VerifiedAt          *time.Time `json:"verifiedAt"`
	ExpiresAt           *time.Time `json:"expiresAt"`
	ApprovedBy          string     `gorm:"size:64;not null;default:''" json:"approvedBy"`
	ApprovedAt          *time.Time `json:"approvedAt"`
	RevokedBy           string     `gorm:"size:64;not null;default:''" json:"revokedBy"`
	RevokedAt           *time.Time `json:"revokedAt"`
	RevokeReason        string     `gorm:"size:255;not null;default:''" json:"revokeReason"`
	TokenJSON           string     `gorm:"type:text" json:"tokenJson"`
	CreatedAt           time.Time  `json:"createdAt"`
}

type DomainTrustPolicy struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	FromDomainCode string    `gorm:"size:64;index;not null" json:"fromDomainCode"`
	ToDomainCode   string    `gorm:"size:64;index;not null" json:"toDomainCode"`
	PolicyLevel    string    `gorm:"size:32;not null;default:'ALLOW'" json:"policyLevel"`
	AllowedPerms   string    `gorm:"size:255;not null;default:'READ'" json:"allowedPerms"`
	AllowedRes     string    `gorm:"size:255;not null;default:'*'" json:"allowedRes"`
	Enabled        bool      `gorm:"not null;default:true" json:"enabled"`
	UpdatedBy      string    `gorm:"size:64;not null;default:''" json:"updatedBy"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type OracleNode struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	NodeName  string    `gorm:"size:64;uniqueIndex;not null" json:"nodeName"`
	PublicKey string    `gorm:"size:255;not null" json:"publicKey"`
	Status    string    `gorm:"size:32;not null;default:'ACTIVE'" json:"status"`
	LastSeen  time.Time `json:"lastSeen"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type SystemSetting struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ConfKey   string    `gorm:"size:64;uniqueIndex;not null" json:"key"`
	ConfValue string    `gorm:"size:255;not null;default:''" json:"value"`
	UpdatedBy string    `gorm:"size:64;not null;default:''" json:"updatedBy"`
	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}

type DeviceStateUpdate struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	DeviceDID     string    `gorm:"size:128;index;not null" json:"deviceDid"`
	Online        bool      `json:"online"`
	FirmwareValid bool      `json:"firmwareValid"`
	CertValid     bool      `json:"certValid"`
	Score         int       `json:"score"`
	StateLabel    string    `gorm:"size:64;index;not null" json:"stateLabel"`
	Severity      int       `gorm:"index;not null" json:"severity"`
	Message       string    `gorm:"size:255;not null" json:"message"`
	TxHash        string    `gorm:"size:128" json:"txHash"`
	BlockHeight   uint64    `json:"blockHeight"`
	CreatedAt     time.Time `json:"createdAt"`
}

type ProtectedOperation struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	DeviceDID  string    `gorm:"size:128;index;not null" json:"deviceDid"`
	DomainCode string    `gorm:"size:64;not null" json:"domainCode"`
	Operation  string    `gorm:"size:64;not null" json:"operation"`
	Payload    string    `gorm:"type:text" json:"payload"`
	CreatedBy  string    `gorm:"size:64;not null" json:"createdBy"`
	CreatedAt  time.Time `json:"createdAt"`
}

type ChainAnchor struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	BizType     string    `gorm:"size:64;not null" json:"bizType"`
	BizRef      string    `gorm:"size:128;index;not null" json:"bizRef"`
	Digest      string    `gorm:"size:128;not null" json:"digest"`
	TxHash      string    `gorm:"size:128;not null" json:"txHash"`
	BlockHeight uint64    `json:"blockHeight"`
	CreatedAt   time.Time `json:"createdAt"`
}

type AuditLog struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Module      string    `gorm:"size:64;index;not null" json:"module"`
	Action      string    `gorm:"size:64;not null" json:"action"`
	Operator    string    `gorm:"size:64;not null" json:"operator"`
	Result      string    `gorm:"size:32;not null" json:"result"`
	Contract    string    `gorm:"size:64;not null;default:''" json:"contract"`
	Method      string    `gorm:"size:64;not null;default:''" json:"method"`
	From        string    `gorm:"size:96;not null;default:''" json:"from"`
	SubjectDID  string    `gorm:"size:128;index;not null;default:''" json:"subjectDid"`
	TxHash      string    `gorm:"size:128;index" json:"txHash"`
	BlockHeight uint64    `json:"blockHeight"`
	GasUsed     uint64    `json:"gasUsed"`
	Message     string    `gorm:"size:255" json:"message"`
	DetailJSON  string    `gorm:"type:text" json:"detailJson"`
	OccurredAt  time.Time `gorm:"autoCreateTime" json:"occurredAt"`
}
