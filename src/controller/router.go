package controller

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"time"
)

func GetRouter(c *Ctrl) *gin.Engine {
	router := gin.Default()
	router.Use(newCorsConfig())
	api := router.Group("api")
	api.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Hello World asu",
		})
	})
	api.GET("nodes", c.GetNodeList())
	api.GET("nodes/:name", c.GetNodeDetail())
	api.GET("pods/:name", c.GetPodDetail())
	return router
}

func newCorsConfig() gin.HandlerFunc {
	config := cors.New(cors.Config{
		// アクセスを許可したいアクセス元
		AllowOrigins: []string{
			"http://localhost:3000",
			"http://10.20.22.192:80",
		},
		// アクセスを許可したいHTTPメソッド(以下の例だとPUTやDELETEはアクセスできません)
		AllowMethods: []string{
			"POST",
			"GET",
			"OPTIONS",
		},
		// 許可したいHTTPリクエストヘッダ
		AllowHeaders: []string{
			"Access-Control-Allow-Credentials",
			"Access-Control-Allow-Headers",
			"Content-Type",
			"Content-Length",
			"Accept-Encoding",
			"Authorization",
		},
		// cookieなどの情報を必要とするかどうか
		AllowCredentials: true,
		// preflightリクエストの結果をキャッシュする時間
		MaxAge: 24 * time.Hour,
	})

	return config
}
