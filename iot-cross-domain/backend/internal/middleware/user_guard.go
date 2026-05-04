package middleware

import (
	"net/http"

	"iot-cross-domain/backend/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// EnsureUserValid checks whether the token user still exists.
// It blocks requests immediately after an account is revoked/deleted.
func EnsureUserValid(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.GetString("username")
		if username == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "invalid token user"})
			return
		}

		var u model.User
		if err := db.Where("username = ?", username).First(&u).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "user revoked or not found"})
			return
		}

		// Refresh role/domain from DB to avoid stale token claims.
		c.Set("userId", u.ID)
		c.Set("role", u.Role)
		c.Set("domainCode", u.DomainCode)
		c.Next()
	}
}

