package controller

import (
	"context"
	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"net/http"
)

type ctrl struct {
	kubeClient *kubernetes.Clientset
}

func NewController(kubeClient *kubernetes.Clientset) *ctrl {
	return &ctrl{
		kubeClient: kubeClient,
	}
}

type PodViewModel struct {
	Name string `json:"name"`
}

type NodeViewModel struct {
	Name     string         `json:"name"`
	TotalPod int            `json:"total_pod"`
	PodList  []PodViewModel `json:"pod_list"`
}

type NodeListViewModel struct {
	TotalNode int             `json:"total_node"`
	NodeList  []NodeViewModel `json:"node_list"`
}

func (c *ctrl) GetNodeList() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		nodes, err := c.kubeClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
		}

		nodeList := make([]NodeViewModel, 0, len(nodes.Items))
		for _, node := range nodes.Items {
			pods, err := c.kubeClient.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
				FieldSelector: "spec.nodeName=" + node.Name,
			})
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{
					"error": err.Error(),
				})
			}

			podList := make([]PodViewModel, 0, len(pods.Items))
			for _, pod := range pods.Items {
				podList = append(podList, PodViewModel{
					Name: pod.Name,
				})
			}
			nodeList = append(nodeList, NodeViewModel{
				Name:     node.Name,
				TotalPod: len(podList),
				PodList:  podList,
			})
		}

		res := NodeListViewModel{
			TotalNode: len(nodeList),
			NodeList:  nodeList,
		}

		ctx.JSON(http.StatusOK, res)
	}
}

func (c *ctrl) GetPodDetail() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		podName := ctx.Param("name")
		_, err := c.kubeClient.CoreV1().Pods("").Get(context.TODO(), podName, metav1.GetOptions{})
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
		}

		//for _, pod := range pods.Items {
		//	fmt.Println(pod.Name)
		//	fmt.Print(">>>> PodIP:")
		//	fmt.Println(pod.Status.PodIP)
		//	fmt.Print(">>>> Namespace:")
		//	fmt.Println(pod.Namespace)
		//	fmt.Print(">>>> labels:")
		//	fmt.Println(pod.Labels)
		//	fmt.Println("")
		//}

		//namespace := "default"
		//pod := "nginx-app1"
		//_, err = clientset.CoreV1().Pods(namespace).Get(context.TODO(), pod, metav1.GetOptions{})
		//if errors.IsNotFound(err) {
		//	fmt.Printf("Pod %s in namespace %s not found\n", pod, namespace)
		//} else if statusError, isStatus := err.(*errors.StatusError); isStatus {
		//	fmt.Printf("Error getting pod %s in namespace %s: %v\n",
		//		pod, namespace, statusError.ErrStatus.Message)
		//} else if err != nil {
		//	panic(err.Error())
		//} else {
		//	fmt.Printf("Found pod %s in namespace %s\n", pod, namespace)
		//}

		ctx.JSON(200, gin.H{
			"message": "Hello World asu",
		})
	}
}

type NodeDetailViewModel struct {
	Name    string `json:"name"`
	Ip      string `json:"ip"`
	PodCidr string `json:"pod_cidr"`
}

func (c *ctrl) GetNodeDetail() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		nodeName := ctx.Param("name")
		node, err := c.kubeClient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
		}

		res := NodeDetailViewModel{
			Name:    node.Name,
			Ip:      node.Status.Addresses[0].Address,
			PodCidr: node.Spec.PodCIDR,
		}

		ctx.JSON(200, res)
	}
}
