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

// RequireCrossDomainAuth ensures sensitive operations only execute after a
// successful cross-domain authentication. The header contract:
//
//   X-Device-DID         — the source device that holds the AuthToken
//                           (i.e. session.device_d_id; the credential
//                            holder that originally proved identity)
//   X-Target-Domain      — the domain this token is allowed to access
//                           (i.e. session.to_domain_code)
//   X-Target-Device-DID  — OPTIONAL: when set, the request operates on
//                           this *target-domain* device instead of the
//                           source device. The target device must
//                           belong to X-Target-Domain and be ACTIVE.
//                           When omitted, falls back to X-Device-DID
//                           so existing clients keep working.
//
// After validation, the resolved "device under operation" is exposed
// via c.Get("targetDeviceDid") so handlers can read it without
// re-parsing headers.
func RequireCrossDomainAuth(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		deviceDID := HeaderDecoded(c, "X-Device-DID")
		targetDomain := HeaderDecoded(c, "X-Target-Domain")
		targetDeviceDID := HeaderDecoded(c, "X-Target-Device-DID")
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

		// Source device must exist + be ACTIVE (the cred holder).
		var srcDevice model.Device
		if err := db.Where("device_d_id = ?", deviceDID).First(&srcDevice).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "device not found"})
			return
		}
		if srcDevice.Lifecycle != "ACTIVE" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "device lifecycle is not ACTIVE"})
			return
		}

		// Resolve "device under operation": prefer X-Target-Device-DID,
		// fall back to X-Device-DID for backwards compatibility.
		opDID := deviceDID
		if targetDeviceDID != "" && targetDeviceDID != deviceDID {
			var tgt model.Device
			if err := db.Where("device_d_id = ?", targetDeviceDID).First(&tgt).Error; err != nil {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "target device not found"})
				return
			}
			if tgt.DomainCode != targetDomain {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "target device not in target domain"})
				return
			}
			if tgt.Lifecycle != "ACTIVE" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "target device lifecycle is not ACTIVE"})
				return
			}
			opDID = targetDeviceDID
		}

		c.Set("crossSrcDeviceDid", deviceDID)
		c.Set("crossOpDeviceDid", opDID)
		c.Next()
	}
}
