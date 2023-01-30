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

func (c *Ctrl) GetNodeList() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		fmt.Println("ノード一覧")
		now := time.Now() // 計測用
		nodeList, err := c.kubeClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		podList, err := c.kubeClient.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		fmt.Printf("kube-api応答時間: %v\n", time.Since(now)) // 計測用

		nodeNamePodsMap := make(map[string][]model.PodViewModel, len(nodeList.Items))
		for _, pod := range podList.Items {
			nodeName := pod.Spec.NodeName
			if _, ok := nodeNamePodsMap[nodeName]; !ok {
				nodeNamePodsMap[nodeName] = make([]model.PodViewModel, 0, len(podList.Items)/len(nodeList.Items))
			}
			nodeNamePodsMap[nodeName] = append(nodeNamePodsMap[nodeName], model.PodViewModel{
				Name: pod.Name,
			})
		}

		nodes := make([]model.NodeViewModel, 0, len(nodeList.Items))
		for _, node := range nodeList.Items {
			nodes = append(nodes, model.NodeViewModel{
				Name:     node.Name,
				TotalPod: len(nodeNamePodsMap[node.Name]),
				Pods:     nodeNamePodsMap[node.Name],
			})
		}

		res := model.NodeListViewModel{
			TotalNode: len(nodes),
			Nodes:     nodes,
		}

		ctx.JSON(http.StatusOK, res)
		fmt.Printf("全体処理時間: %v\n", time.Since(now)) // 計測用
	}
}
