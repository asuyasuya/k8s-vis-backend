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
		now := time.Now() // 計測用
		nodes, err := c.kubeClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		pods, err := c.kubeClient.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		nodeNamePodListMap := make(map[string][]model.PodViewModel, len(nodes.Items))
		for _, pod := range pods.Items {
			nodeName := pod.Spec.NodeName
			if _, ok := nodeNamePodListMap[nodeName]; !ok {
				nodeNamePodListMap[nodeName] = make([]model.PodViewModel, len(pods.Items)/len(nodes.Items))
			}
			nodeNamePodListMap[nodeName] = append(nodeNamePodListMap[nodeName], model.PodViewModel{
				Name: pod.Name,
			})
		}

		nodeList := make([]model.NodeViewModel, 0, len(nodes.Items))
		for _, node := range nodes.Items {
			nodeList = append(nodeList, model.NodeViewModel{
				Name:     node.Name,
				TotalPod: len(nodeNamePodListMap[node.Name]),
				PodList:  nodeNamePodListMap[node.Name],
			})
		}

		res := model.NodeListViewModel{
			TotalNode: len(nodeList),
			NodeList:  nodeList,
		}

		ctx.JSON(http.StatusOK, res)
		fmt.Printf("経過: %vms\n", time.Since(now).Milliseconds()) // 計測用
	}
}
