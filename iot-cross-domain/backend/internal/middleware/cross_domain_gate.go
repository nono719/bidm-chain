package middleware

import (
	"net/http"
	"net/url"
	"time"

	"iot-cross-domain/backend/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// HeaderDecoded reads a header and URL-decodes it so non-ASCII values
// (e.g. Chinese domain codes) sent as %XX-encoded bytes survive the trip.
func HeaderDecoded(c *gin.Context, key string) string {
	v := c.GetHeader(key)
	if decoded, err := url.QueryUnescape(v); err == nil {
		return decoded
	}
	return v
}

// RequireCrossDomainAuth ensures sensitive operations only execute after successful cross-domain authentication.
func RequireCrossDomainAuth(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		deviceDID := HeaderDecoded(c, "X-Device-DID")
		targetDomain := HeaderDecoded(c, "X-Target-Domain")
		if deviceDID == "" || targetDomain == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"message": "missing X-Device-DID or X-Target-Domain"})
			return
		}

		var session model.CrossDomainAuthSession
		err := db.Where("device_d_id = ? AND to_domain_code = ? AND status = ?", deviceDID, targetDomain, "VERIFIED").
			Order("verified_at desc").
			First(&session).Error
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "cross-domain authentication required"})
			return
		}

		if session.ExpiresAt != nil {
			if time.Now().After(*session.ExpiresAt) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "cross-domain authentication expired"})
				return
			}
		} else {
			if session.VerifiedAt == nil || time.Since(*session.VerifiedAt) > 30*time.Minute {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "cross-domain authentication expired"})
				return
			}
		}

		var device model.Device
		if err := db.Where("device_d_id = ?", deviceDID).First(&device).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "device not found"})
			return
		}
		if device.Lifecycle != "ACTIVE" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "device lifecycle is not ACTIVE"})
			return
		}

		c.Next()
	}
}
