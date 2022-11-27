package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"log"
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

func findPodByName(list *v1.PodList, name string) (v1.Pod, error) {
	if list == nil || len(list.Items) == 0 {
		return v1.Pod{}, errors.New("ListItems is empty")
	}

	for _, pod := range list.Items {
		if pod.Name == name {
			return pod, nil
		}
	}

	return v1.Pod{}, errors.New("ListItems is empty")
}

func hasIngress(types []netv1.PolicyType) bool {
	for _, v := range types {
		if v == netv1.PolicyTypeIngress {
			return true
		}
	}
	return false
}

func hasEgress(types []netv1.PolicyType) bool {
	for _, v := range types {
		if v == netv1.PolicyTypeEgress {
			return true
		}
	}
	return false
}

func podIsRelated(policy netv1.NetworkPolicy, pod v1.Pod) bool {
	if policy.Namespace != pod.Namespace {
		return false
	}

	// matchLabelとmatchExpressionは全てAND条件である。　一つでも条件を満たさなかったらfalseを返す。
	// matchLabel(等価ベース map)について
	if policy.Spec.PodSelector.MatchLabels != nil {
		for k, v := range policy.Spec.PodSelector.MatchLabels {
			if pod.Labels[k] != v {
				fmt.Println("くるはずapp3: ", v)
				return false
			}
		}
	}
	// matchExpressions(集合ベース 構造体の配列)について
	if policy.Spec.PodSelector.MatchExpressions != nil {
		for _, v := range policy.Spec.PodSelector.MatchExpressions {
			switch v.Operator {
			case metav1.LabelSelectorOpIn:
				in := false
				for _, v2 := range v.Values {
					if pod.Labels[v.Key] == v2 {
						in = true
					}
				}
				if !in {
					return false
				}
			case metav1.LabelSelectorOpNotIn:
				notIn := false
				for _, v2 := range v.Values {
					if pod.Labels[v.Key] != "" && pod.Labels[v.Key] != v2 {
						notIn = true
					}
				}
				if !notIn {
					return false
				}
			case metav1.LabelSelectorOpExists:
				if pod.Labels[v.Key] == "" {
					return false
				}
			case metav1.LabelSelectorOpDoesNotExist:
				if pod.Labels[v.Key] != "" {
					return false
				}
			}
		}
	}

	return true
}

func filterPolicyListByPod(policyListItems []netv1.NetworkPolicy, pod v1.Pod) (filteredPolicyListItems []netv1.NetworkPolicy) {
	for _, policy := range policyListItems {
		if podIsRelated(policy, pod) {
			filteredPolicyListItems = append(filteredPolicyListItems, policy)
		}
	}

	return filteredPolicyListItems
}

func (c *ctrl) GetPodDetail() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		podName := ctx.Param("name")
		podList, err := c.kubeClient.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})

		pod, err := findPodByName(podList, podName)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		if len(podList.Items) == 0 {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": "There is not a pod named" + podName,
			})
			return
		}

		// ネットワークポリシー一覧を取得する
		policyList, err := c.kubeClient.NetworkingV1().NetworkPolicies("").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		// target pod のポリシー1を探す ingress egress両方
		filteredPolicyListItems := filterPolicyListByPod(policyList.Items, pod)

		log.Println("filteredPolicyListItems: ", filteredPolicyListItems[0])
		log.Println("+++++++++++++++++length:", len(filteredPolicyListItems))

		// ポリシー1で許可してあるpodリストを取得。 ingress egress 両方
		for _, pod := range podList.Items {
			for _, policy := range filteredPolicyListItems {
				if hasIngress(policy.Spec.PolicyTypes) {
					// podがsrcPodとなりうるかチェックする
					for _, rule := range policy.Spec.Ingress {
						// or
						for _, peer := range rule.From {
							// and
							if peer.NamespaceSelector == nil {
								// ネットワークポリシーが属するnamespaceが対象になる

							} else {
								// peer.NamespaceSelectorで選択したnamespaceが対象になる。
							}
						}
					}
				}
			}
		}
		//各々のpodについてそのpodのポリシー2を探す。一回のループでingress egress 両方
		// ポリシー2ではtarget podへのegress(ingress)が許可されているか
		// egressが一つでもあり、target podに関してはない→許可しない
		// egressが一つもない→　許可する
		// target pod に関するegresssがある

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
			return
		}

		res := NodeDetailViewModel{
			Name:    node.Name,
			Ip:      node.Status.Addresses[0].Address,
			PodCidr: node.Spec.PodCIDR,
		}

		ctx.JSON(200, res)
	}
}
