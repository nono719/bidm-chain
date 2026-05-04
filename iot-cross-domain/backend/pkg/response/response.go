package response

import "github.com/gin-gonic/gin"

func OK(c *gin.Context, data any) {
	c.JSON(200, gin.H{"code": 0, "message": "ok", "data": data})
}

func BadRequest(c *gin.Context, msg string) {
	c.JSON(400, gin.H{"code": 400, "message": msg})
}

func Unauthorized(c *gin.Context, msg string) {
	c.JSON(401, gin.H{"code": 401, "message": msg})
}

func Forbidden(c *gin.Context, msg string) {
	c.JSON(403, gin.H{"code": 403, "message": msg})
}

func InternalError(c *gin.Context, msg string) {
	c.JSON(500, gin.H{"code": 500, "message": msg})
}
