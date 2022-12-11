package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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

type Label struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type PortInfo struct {
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	EndPort  int    `json:"end_port"`
}

type PodPolicy struct {
	Name      string     `json:"name"`
	CanAccess bool       `json:"can_access"`
	Ports     []PortInfo `json:"ports"`
}

type PodDetailViewModel struct {
	Name      string      `json:"name"`
	Ip        string      `json:"ip"`
	Namespace string      `json:"namespace"`
	Labels    []Label     `json:"labels"`
	Ingress   []PodPolicy `json:"ingress"`
	Egress    []PodPolicy `json:"egress"`
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

func getNamespaceMap(namespaceListItems []v1.Namespace) map[string]v1.Namespace {
	m := make(map[string]v1.Namespace, len(namespaceListItems))
	for _, v := range namespaceListItems {
		m[v.Name] = v
	}
	return m
}

func checkProtocol(targetProtocol *v1.Protocol, otherProtocol *v1.Protocol) (*v1.Protocol, bool) {
	if targetProtocol == nil && otherProtocol == nil {
		return nil, true
	}

	if targetProtocol == nil && otherProtocol != nil {
		return otherProtocol, true
	}

	if targetProtocol != nil && otherProtocol == nil {
		return targetProtocol, true
	}

	if *targetProtocol == *otherProtocol {
		return targetProtocol, true
	}

	return nil, false
}

func checkPort(targetPort netv1.NetworkPolicyPort, otherPort netv1.NetworkPolicyPort) (*intstr.IntOrString, *int32, bool) {
	if targetPort.Port == nil && otherPort.Port == nil {
		return nil, nil, true
	}

	if targetPort.Port == nil && otherPort.Port != nil {
		return otherPort.Port, otherPort.EndPort, true
	}

	if targetPort.Port != nil && otherPort.Port == nil {
		return targetPort.Port, targetPort.EndPort, true
	}

	// EndPort絡み
	if targetPort.EndPort == nil && otherPort.EndPort == nil {
		if targetPort.Port.IntVal == otherPort.Port.IntVal {
			return targetPort.Port, targetPort.EndPort, true
		}
		return nil, nil, false
	}

	if targetPort.EndPort == nil && otherPort.EndPort != nil {
		if otherPort.Port.IntVal <= targetPort.Port.IntVal && targetPort.Port.IntVal <= *otherPort.EndPort {
			return targetPort.Port, nil, true
		}
		return nil, nil, false
	}

	if targetPort.EndPort != nil && otherPort.EndPort == nil {
		if targetPort.Port.IntVal <= otherPort.Port.IntVal && otherPort.Port.IntVal <= *targetPort.EndPort {
			return otherPort.Port, nil, true
		}
		return nil, nil, false
	}

	// どっちもendあり
	if otherPort.Port.IntVal < targetPort.Port.IntVal {
		if targetPort.Port.IntVal <= *otherPort.EndPort && *otherPort.EndPort <= *targetPort.EndPort {
			return targetPort.Port, otherPort.EndPort, true
		}

		if *targetPort.EndPort <= *otherPort.EndPort {
			return targetPort.Port, targetPort.EndPort, true
		}

		return nil, nil, false
	} else {
		if *otherPort.EndPort <= *targetPort.EndPort {
			return otherPort.Port, otherPort.EndPort, true
		}

		if *targetPort.EndPort <= *otherPort.EndPort {
			return otherPort.Port, targetPort.EndPort, true
		}

		return nil, nil, false
	}
}

func getAccessPorts(targetPorts []netv1.NetworkPolicyPort, otherPorts []netv1.NetworkPolicyPort) ([]netv1.NetworkPolicyPort, bool) {
	if len(targetPorts) == 0 && len(otherPorts) == 0 {
		return nil, true
	}

	if len(targetPorts) == 0 && len(otherPorts) != 0 {
		return otherPorts, true
	}

	if len(targetPorts) != 0 && len(otherPorts) == 0 {
		return targetPorts, true
	}

	var res []netv1.NetworkPolicyPort
	for _, targetPort := range targetPorts {
		for _, otherPort := range otherPorts {
			// protocolのチェック
			protocol, ok := checkProtocol(targetPort.Protocol, otherPort.Protocol)
			if !ok {
				continue
			}

			// portのチェック
			port, endPort, ok2 := checkPort(targetPort, otherPort)
			if !ok2 {
				continue
			}

			res = append(res, netv1.NetworkPolicyPort{
				Protocol: protocol,
				Port:     port,
				EndPort:  endPort,
			})
		}
	}

	return res, len(res) != 0
}

func castProtocol(protocol *v1.Protocol) string {
	if protocol == nil {
		return "any"
	}

	switch *protocol {
	case v1.ProtocolTCP:
		return "TCP"
	case v1.ProtocolUDP:
		return "UDP"
	case v1.ProtocolSCTP:
		return "SCTP"
	default:
		return "unknown"
	}
}

type policyInfo struct {
	pod   v1.Pod
	ports []netv1.NetworkPolicyPort
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
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		namespaceMap := getNamespaceMap(namespaceList.Items)

		// todo ちゃんとこの段階でingress空の時に全てのPodが入っているようにする
		// todo ingress{} と　未選択によりすべてDenyを判別できるようにする。
		targetIngressMap, targetEgressMap, podMap := getTargetPodPolicyMap(ctx, targetPod, podList, namespaceMap, policyList)

		var res PodDetailViewModel
		var ingressPodPolicyMap map[string]PodPolicy
		var egressPodPolicyMap map[string]PodPolicy

		for _, v := range podMap {
			destIngressMap, srcEgressMap, _ := getTargetPodPolicyMap(ctx, v, podList, namespaceMap, policyList)
			// ingressMap[v.Name]とsrcEgressMap[targetPod]の交わりが行けるポート番号
			targetIngressPolicy, ok1 := targetIngressMap[v.Name]
			srcEgressPolicy, ok2 := srcEgressMap[targetPod.Name]
			if ok1 && ok2 {
				if accessPorts, hasPort := getAccessPorts(targetIngressPolicy.ports, srcEgressPolicy.ports); hasPort {
					var resPorts []PortInfo
					for _, port := range accessPorts {
						resPorts = append(resPorts, PortInfo{
							Protocol: castProtocol(port.Protocol),
							Port:     int(port.Port.IntVal),
							EndPort:  int(*port.EndPort),
						})
					}
					ingressPodPolicyMap[v.Name] = PodPolicy{
						Name:      v.Name,
						CanAccess: true,
						Ports:     resPorts,
					}
				}
			}

			// egressMap[v.Name]とdestIngressMap[targetPod]の交わりがおっけ
			targetEgressPolicy, ok1 := targetEgressMap[v.Name]
			destIngressPolicy, ok2 := destIngressMap[targetPod.Name]
			if ok1 && ok2 {
				if accessPorts, hasPort := getAccessPorts(targetEgressPolicy.ports, destIngressPolicy.ports); hasPort {
					var resPorts []PortInfo
					for _, port := range accessPorts {
						resPorts = append(resPorts, PortInfo{
							Protocol: castProtocol(port.Protocol),
							Port:     int(port.Port.IntVal),
							EndPort:  int(*port.EndPort),
						})
					}
					egressPodPolicyMap[v.Name] = PodPolicy{
						Name:      v.Name,
						CanAccess: true,
						Ports:     resPorts,
					}
				}
			}
		}

		var ingress []PodPolicy
		var egress []PodPolicy

		for _, pod := range podList.Items {
			if policy, ok := ingressPodPolicyMap[pod.Name]; ok {
				ingress = append(ingress, policy)
			} else {
				ingress = append(ingress, PodPolicy{
					Name:      pod.Name,
					CanAccess: false,
				})
			}

			if policy, ok := egressPodPolicyMap[pod.Name]; ok {
				egress = append(egress, policy)
			} else {
				egress = append(egress, PodPolicy{
					Name:      pod.Name,
					CanAccess: false,
				})
			}
		}

		var resLabels []Label
		for k, v := range targetPod.Labels {
			resLabels = append(resLabels, Label{
				Key:   k,
				Value: v,
			})
		}

		res.Name = targetPod.Name
		res.Namespace = targetPod.Namespace
		res.Ip = targetPod.Status.PodIP
		res.Labels = resLabels
		res.Ingress = ingress
		res.Egress = egress

		ctx.JSON(http.StatusOK, res)
	}
}

func getTargetPodPolicyMap(ctx *gin.Context, targetPod v1.Pod, podList *v1.PodList, namespaceMap map[string]v1.Namespace, policyList *netv1.NetworkPolicyList) (map[string]*policyInfo, map[string]*policyInfo, map[string]v1.Pod) {
	// target pod のポリシー1を探す ingress egress両方
	filteredPolicyListItems := filterPolicyListByPod(policyList.Items, targetPod)

	ingressMap := make(map[string]*policyInfo, len(policyList.Items))
	egressMap := make(map[string]*policyInfo, len(policyList.Items))
	podMap := make(map[string]v1.Pod, len(policyList.Items))
	// ポリシー1で許可してあるpodリストを取得。 ingress egress 両方
	for _, pod := range podList.Items {
		if targetPod.Name == pod.Name {
			// 自身は除外
			continue
		}
		namespace := namespaceMap[pod.Namespace]
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
							return nil, nil, nil
						}

						if !isIncluded {
							continue
						}

						if _, ok := ingressMap[pod.Name]; !ok {
							ingressMap[pod.Name] = &policyInfo{
								pod:   pod,
								ports: make([]netv1.NetworkPolicyPort, 0, 5),
							}
						}
						ingressMap[pod.Name].ports = append(ingressMap[pod.Name].ports, rule.Ports...)
						podMap[pod.Name] = pod
					}
				}
			} else if hasEgress(policy.Spec.PolicyTypes) {
				// podがsrcPodとなりうるかチェックする
				for _, rule := range policy.Spec.Egress {
					// or
					for _, peer := range rule.To {
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
							return nil, nil, nil
						}

						if !isIncluded {
							continue
						}

						if _, ok := egressMap[pod.Name]; !ok {
							egressMap[pod.Name] = &policyInfo{
								pod:   pod,
								ports: make([]netv1.NetworkPolicyPort, 0, 5),
							}
						}
						egressMap[pod.Name].ports = append(egressMap[pod.Name].ports, rule.Ports...)
						podMap[pod.Name] = pod
					}
				}
			}
		}
	}
	return ingressMap, egressMap, podMap
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
				log.Println("ports", rule.Ports)
				log.Println("======")
			}
			log.Println("++++++++++")

		}
		ctx.JSON(200, gin.H{
			"message": "OK",
		})
	}
}
