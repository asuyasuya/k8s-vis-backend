package controller

import (
	"context"
	"fmt"
	"github.com/asuyasuya/k8s-vis-backend/src/model"
	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

func (c *Ctrl) GetNodeDetail() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		fmt.Println("ノード詳細")
		now := time.Now()
		nodeName := ctx.Param("name")
		node, err := c.kubeClient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		fmt.Printf("k8sAPIレイテンシー: %v\n", time.Since(now)) // 計測用

		res := model.NodeDetailViewModel{
			Name:    node.Name,
			Ip:      node.Status.Addresses[0].Address,
			PodCidr: node.Spec.PodCIDR,
		}

		ctx.JSON(200, res)
		fmt.Printf("可視化システム処理時間: %v\n", time.Since(now)) // 計測用
	}
}
