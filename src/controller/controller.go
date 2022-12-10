package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"net"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"log"
	"net/http"
)

type Ctrl struct {
	kubeClient *kubernetes.Clientset
}

func NewController(kubeClient *kubernetes.Clientset) *Ctrl {
	return &Ctrl{
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

func (c *Ctrl) GetNodeList() gin.HandlerFunc {
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

func findNamespaceByName(list *v1.NamespaceList, name string) (v1.Namespace, error) {
	if list == nil || len(list.Items) == 0 {
		return v1.Namespace{}, errors.New("list items is empty")
	}

	for _, namespace := range list.Items {
		if namespace.Name == name {
			return namespace, nil
		}
	}

	return v1.Namespace{}, errors.New("cannot find the namespace")
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

func isIncludedInLabelSelector(labels map[string]string, podSelector *metav1.LabelSelector) bool {

	// matchLabelとmatchExpressionは全てAND条件である。　一つでも条件を満たさなかったらfalseを返す。
	// matchLabel(等価ベース map)について
	if podSelector.MatchLabels != nil {
		for k, v := range podSelector.MatchLabels {
			if labels[k] != v {
				fmt.Println("くるはずapp3: ", v)
				return false
			}
		}
	}
	// matchExpressions(集合ベース 構造体の配列)について
	if podSelector.MatchExpressions != nil {
		for _, v := range podSelector.MatchExpressions {
			switch v.Operator {
			case metav1.LabelSelectorOpIn:
				in := false
				for _, v2 := range v.Values {
					if labels[v.Key] == v2 {
						in = true
					}
				}
				if !in {
					return false
				}
			case metav1.LabelSelectorOpNotIn:
				notIn := false
				for _, v2 := range v.Values {
					if labels[v.Key] != "" && labels[v.Key] != v2 {
						notIn = true
					}
				}
				if !notIn {
					return false
				}
			case metav1.LabelSelectorOpExists:
				if labels[v.Key] == "" {
					return false
				}
			case metav1.LabelSelectorOpDoesNotExist:
				if labels[v.Key] != "" {
					return false
				}
			}
		}
	}

	// todo podセレクターがから{}の時にちゃんとtrueになることを確認

	return true
}

func filterPolicyListByPod(policyListItems []netv1.NetworkPolicy, pod v1.Pod) (filteredPolicyListItems []netv1.NetworkPolicy) {
	for _, policy := range policyListItems {
		if policy.Namespace == pod.Namespace && isIncludedInLabelSelector(pod.Labels, &policy.Spec.PodSelector) {
			filteredPolicyListItems = append(filteredPolicyListItems, policy)
		}
	}

	return filteredPolicyListItems
}

func isIncludedInIpBlock(ipBlock *netv1.IPBlock, ip string) (bool, error) {
	if ipBlock == nil {
		// ipBlockに関する条件がないのでtrueで返す
		return true, nil
	}

	_, cidrNet, err := net.ParseCIDR(ipBlock.CIDR)
	if err != nil {
		log.Fatal(err)
		return false, err
	}

	targetIP := net.ParseIP(ip)
	if !cidrNet.Contains(targetIP) {
		return false, nil
	}

	if len(ipBlock.Except) > 0 {
		for _, v := range ipBlock.Except {
			_, exceptCidrNet, err := net.ParseCIDR(v)
			if err != nil {
				return false, err
			}
			if exceptCidrNet.Contains(targetIP) {
				return false, nil
			}
		}
	}

	return true, nil
}

func (c *Ctrl) GetPodDetail() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		podName := ctx.Param("name")
		podList, err := c.kubeClient.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})

		targetPod, err := findPodByName(podList, podName)
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
		filteredPolicyListItems := filterPolicyListByPod(policyList.Items, targetPod)

		log.Println("filteredPolicyListItems: ", filteredPolicyListItems[0])
		log.Println("+++++++++++++++++length:", len(filteredPolicyListItems))

		// namespaceSelectorで選択する必要があるので、namespace一覧を取得する
		namespaceList, err := c.kubeClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})

		var ingressSrcPodList []v1.Pod
		//var egressDestPodList []v1.Pod

		// ポリシー1で許可してあるpodリストを取得。 ingress egress 両方
		for _, pod := range podList.Items {
			if targetPod.Name == pod.Name {
				// 自身は除外
				continue
			}
			for _, policy := range filteredPolicyListItems {
				// 指定がない時はingressになるか　→ なる
				if hasIngress(policy.Spec.PolicyTypes) {
					// podがsrcPodとなりうるかチェックする
					for _, rule := range policy.Spec.Ingress {
						// or
						for _, peer := range rule.From {
							// and
							if peer.NamespaceSelector == nil {
								// namespaceSelectorが与えられない時と 与えられるが空だった時の違い
								// → 与えれらない時はnullになって、与えられるが空になる時は構造体が空になる
								// ネットワークポリシーが属するnamespaceが対象になる

								// namespaceのチェック
								if policy.Namespace != pod.Namespace {
									continue
								}

							} else {
								// podの属しているnamespaceのlabelsがpeer.NamespaceSelectorに含まれている場合行ける

								// namespaceのチェック
								namespace, err := findNamespaceByName(namespaceList, pod.Namespace)
								if err != nil {
									ctx.JSON(http.StatusInternalServerError, gin.H{
										"error": "invalid ip block",
									})
									return
								}
								if !isIncludedInLabelSelector(namespace.Labels, peer.NamespaceSelector) {
									continue
								}

							}
							// PodSelectorのチェック
							if !isIncludedInLabelSelector(pod.Labels, peer.PodSelector) {
								continue
							}

							// podcidrのチェック
							isIncluded, err := isIncludedInIpBlock(peer.IPBlock, pod.Status.PodIP)
							if err != nil {
								ctx.JSON(http.StatusInternalServerError, gin.H{
									"error": "invalid ip block",
								})
								return
							}

							if !isIncluded {
								continue
							}

							ingressSrcPodList = append(ingressSrcPodList, pod)
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

func (c *Ctrl) GetNodeDetail() gin.HandlerFunc {
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

func (c *Ctrl) GetTest() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		policy, err := c.kubeClient.NetworkingV1().NetworkPolicies("default").List(context.TODO(), metav1.ListOptions{
			FieldSelector: "metadata.name=pod-selector-test"})
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		log.Println("policy types = ", policy.Items[0].Spec.PolicyTypes)
		log.Println("has ingress = ", hasIngress(policy.Items[0].Spec.PolicyTypes))
		log.Println("has egress = ", hasEgress(policy.Items[0].Spec.PolicyTypes))
		for _, v := range policy.Items {
			log.Println("++++++++++")
			log.Println("ingress rule length = ", len(v.Spec.Ingress))
			for _, rule := range v.Spec.Ingress {
				log.Println("======")
				log.Println("ingress peer length = ", len(rule.From))
				for _, peer := range rule.From {
					log.Println("namespaceSelector = ", peer.NamespaceSelector)
					log.Println("namespaceSelector is null = ", peer.NamespaceSelector == nil)
					log.Println("podSelector = ", peer.PodSelector)
					log.Println("ipBlock = ", peer.IPBlock)
					log.Println("---")
				}
				log.Println("======")
			}
			log.Println("++++++++++")

		}
		ctx.JSON(200, gin.H{
			"message": "OK",
		})
	}
}
