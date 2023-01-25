package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/asuyasuya/k8s-vis-backend/src/model"
	"github.com/gin-gonic/gin"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
	"net"
	"net/http"
	"time"
)

func (c *Ctrl) GetPodDetail() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		now := time.Now()
		podName := ctx.Param("name")

		// Pod一覧を取得
		podList, err := c.kubeClient.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})

		// Network Policy一覧を取得する
		policyList, err := c.kubeClient.NetworkingV1().NetworkPolicies("").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		// namespaceSelectorで選択する必要があるので、namespace一覧を取得する
		namespaceList, err := c.kubeClient.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

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

		accessPods, policyNames, err := getAccessPods(podList, policyList, namespaceList, targetPod)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": "invalid ip block",
			})
			return
		}

		var res model.PodDetailViewModel
		res.Name = targetPod.Name
		res.Namespace = targetPod.Namespace
		res.Ip = targetPod.Status.PodIP
		res.Labels = model.LabelViewModel(targetPod)
		res.PolicyNames = policyNames
		res.AccessPods = accessPods

		ctx.JSON(http.StatusOK, res)
		fmt.Printf("経過: %vms\n", time.Since(now).Milliseconds())
	}
}
func getAccessPods(podList *v1.PodList, policyList *netv1.NetworkPolicyList, namespaceList *v1.NamespaceList, targetPod v1.Pod) ([]model.AccessPod, []string, error) {
	namespaceMap := getNamespaceMap(namespaceList.Items)

	// targetPodに適用されたNetwork Policyの一覧を取得
	filteredPolicyList := filterPolicyListByPod(policyList.Items, targetPod)
	// ingressが書かれたpolicyとegressが書かれたpolicyに分ける(どちらの記述もある場合はどちらの配列にも)
	ingressPolicyList, egressPolicyList := classifyIngressOrEgress(filteredPolicyList)
	// policyの名前一覧をを返却用に作る
	policyNames := make([]string, 0, len(filteredPolicyList))
	for _, v := range filteredPolicyList {
		policyNames = append(policyNames, v.Name)
	}

	accessPods := make([]model.AccessPod, len(podList.Items))
	for i, pod := range podList.Items {
		if pod.Name == targetPod.Name {
			// 自身は常にいかなるポートでも通信可
			accessPods[i].Name = pod.Name
			accessPods[i].Ip = pod.Status.PodIP
			accessPods[i].Labels = model.LabelViewModel(pod)
			accessPods[i].Namespace = pod.Namespace
			accessPods[i].Ingress = model.PodPolicy{
				CanAccess: true,
				Ports:     nil,
			}
			accessPods[i].Egress = model.PodPolicy{
				CanAccess: true,
				Ports:     nil,
			}
			continue
		}

		PodNamespace := namespaceMap[pod.Namespace]
		// PodはtargetPodのingressを満たすか、満たすのであればどんなportか取得
		targetIngressPorts, targetIngressOk, err := getIngressPorts(pod, ingressPolicyList, PodNamespace)
		if err != nil {
			return nil, nil, err
		}

		// PodはtargetPodのegressを満たすか、満たすのであればどんなportか取得
		targetEgressPorts, targetEgressOk, err := getEgressPorts(pod, egressPolicyList, PodNamespace)
		if err != nil {
			return nil, nil, err
		}

		// Podに適用されたNetwork Policyの一覧をフィルタリング
		filteredPodPolicyList := filterPolicyListByPod(policyList.Items, pod)

		// ingressが書かれたpolicyとegressが書かれたpolicyに分ける(どちらの記述もある場合はどちらの配列にも)
		podIngressPolicyList, podEgressPolicyList := classifyIngressOrEgress(filteredPodPolicyList)
		targetNamespace := namespaceMap[targetPod.Namespace]

		// PodからtargetPodへのEgressが可能かチェック + 一致しているポートのチェック
		ingressPodPolicy, err := getIngressPodPolicy(targetIngressOk, targetPod, podEgressPolicyList, targetNamespace, targetIngressPorts)
		if err != nil {
			return nil, nil, err
		}

		// targetPodからPodへのIngressが可能かチェック + 一致しているポートのチェック
		egressPodPolicy, err := getEgressPodPolicy(targetEgressOk, targetPod, podIngressPolicyList, targetNamespace, targetEgressPorts)
		if err != nil {
			return nil, nil, err
		}

		accessPods[i].Name = pod.Name
		accessPods[i].Ip = pod.Status.PodIP
		accessPods[i].Labels = model.LabelViewModel(pod)
		accessPods[i].Namespace = pod.Namespace
		accessPods[i].Ingress = ingressPodPolicy
		accessPods[i].Egress = egressPodPolicy
	}

	return accessPods, policyNames, nil
}

func getIngressPodPolicy(targetIngressOk bool, targetPod v1.Pod, podEgressPolicyList []netv1.NetworkPolicy, targetNamespace v1.Namespace, targetIngressPorts []netv1.NetworkPolicyPort) (model.PodPolicy, error) {
	var res model.PodPolicy
	if !targetIngressOk {
		return res, nil
	}

	// targetPodがPodからのingressが可能な時に今度は逆にPodからtargetPodへのEgressが可能かチェックする
	podEgressPorts, podEgressOk, err := getEgressPorts(targetPod, podEgressPolicyList, targetNamespace)
	if err != nil {
		return model.PodPolicy{}, err
	}

	if !podEgressOk {
		return res, nil
	}

	// 重複しているポート情報を削除
	arrangedTargetIngressPorts := deleteDuplicationPolicyPort(targetIngressPorts)
	arrangedPodEgressPorts := deleteDuplicationPolicyPort(podEgressPorts)
	// お互いの許可するポート情報で一致する部分を抽出
	accessPorts, hasPort := getAccessPorts(arrangedTargetIngressPorts, arrangedPodEgressPorts)

	if !hasPort {
		return res, nil
	}

	ports := make([]model.PortInfo, 0, len(accessPorts))
	for _, p := range accessPorts {
		ports = append(ports, model.Cast2PortInfo(p))
	}
	res.CanAccess = true
	res.Ports = ports

	return res, nil
}

func getEgressPodPolicy(targetEgressOk bool, targetPod v1.Pod, podIngressPolicyList []netv1.NetworkPolicy, targetNamespace v1.Namespace, targetEgressPorts []netv1.NetworkPolicyPort) (model.PodPolicy, error) {
	var res model.PodPolicy
	if !targetEgressOk {
		return res, nil
	}

	// targetPodがPodへのEgressが可能な時に今度は逆にPodがtargetPodからのIngressが可能かチェックする
	podIngressPorts, podIngressOk, err := getIngressPorts(targetPod, podIngressPolicyList, targetNamespace)
	if err != nil {
		return model.PodPolicy{}, err
	}

	if !podIngressOk {
		return res, nil
	}

	// 重複しているポート情報を削除
	arrangedTargetEgressPorts := deleteDuplicationPolicyPort(targetEgressPorts)
	arrangedPodIngressPorts := deleteDuplicationPolicyPort(podIngressPorts)
	// お互いの許可するポート情報で一致する部分を抽出
	accessPorts, hasPort := getAccessPorts(arrangedTargetEgressPorts, arrangedPodIngressPorts)

	if !hasPort {
		return res, nil
	}

	ports := make([]model.PortInfo, 0, len(accessPorts))
	for _, p := range accessPorts {
		ports = append(ports, model.Cast2PortInfo(p))
	}
	res.CanAccess = true
	res.Ports = ports

	return res, nil
}

func classifyIngressOrEgress(policyList []netv1.NetworkPolicy) ([]netv1.NetworkPolicy, []netv1.NetworkPolicy) {
	ingressPolicyList := make([]netv1.NetworkPolicy, 0, len(policyList))
	egressPolicyList := make([]netv1.NetworkPolicy, 0, len(policyList))
	for _, p := range policyList {
		if hasIngress(p.Spec.PolicyTypes) {
			ingressPolicyList = append(ingressPolicyList, p)
		}

		if hasIngress(p.Spec.PolicyTypes) {
			egressPolicyList = append(egressPolicyList, p)
		}
	}
	return ingressPolicyList, egressPolicyList
}

func getIngressPorts(srcPod v1.Pod, targetPodIngressPolicyList []netv1.NetworkPolicy, srcPodNamespace v1.Namespace) ([]netv1.NetworkPolicyPort, bool, error) {
	if len(targetPodIngressPolicyList) == 0 {
		// ingressに関してはpolicyによって制限されることがないのでtargetPodは全podの全portで通信可能
		return []netv1.NetworkPolicyPort{}, true, nil
	}

	// 何かしらpolicyが適用されるということ
	ingressPorts := make([]netv1.NetworkPolicyPort, 0, 10)
	ok := false
	for _, policy := range targetPodIngressPolicyList {
		for _, rule := range policy.Spec.Ingress {
			if len(rule.From) == 0 {
				// ingress: {} パターン．全namespaceの全podを受け入れる
				ingressPorts = append(ingressPorts, rule.Ports...)
				ok = true
				continue
			}

			// ここからor条件
			for _, peer := range rule.From {
				// ここからand条件

				// NamespaceSelectorのチェック
				if peer.NamespaceSelector == nil {
					// Network Policyが属するNamespaceに属するPodが対象になる
					if policy.Namespace != srcPod.Namespace {
						continue
					}
				} else {
					// 通信を受け入れるnamespaceが指定されている場合
					// srcPodが属しているnamespaceがpeer.NamespaceSelectorにマッチしているかチェックする
					if !isIncludedInLabelSelector(srcPodNamespace.Labels, peer.NamespaceSelector) {
						continue
					}
				}

				// PodSelectorのチェック
				if !isIncludedInLabelSelector(srcPod.Labels, peer.PodSelector) {
					continue
				}

				// IPBlockのチェック
				isIncluded, err := isIncludedInIpBlock(peer.IPBlock, srcPod.Status.PodIP)
				if err != nil {
					return nil, false, err
				}
				if !isIncluded {
					continue
				}

				// ここまできたらsrcPodはingressに含まれる
				ingressPorts = append(ingressPorts, rule.Ports...)
				ok = true
			}
		}
	}

	return ingressPorts, ok, nil
}

func getEgressPorts(destPod v1.Pod, targetPodEgressPolicyList []netv1.NetworkPolicy, destPodNamespace v1.Namespace) ([]netv1.NetworkPolicyPort, bool, error) {
	if len(targetPodEgressPolicyList) == 0 {
		// egressに関してはpolicyによって制限されることがないのでtargetPodは全podの全portで通信可能
		return []netv1.NetworkPolicyPort{}, true, nil
	}

	// 何かしらpolicyが適用されるということ
	egressPorts := make([]netv1.NetworkPolicyPort, 0, 10)
	ok := false
	for _, policy := range targetPodEgressPolicyList {
		for _, rule := range policy.Spec.Egress {
			if len(rule.To) == 0 {
				// egress: {} パターン．全namespaceの全podへの通信可
				egressPorts = append(egressPorts, rule.Ports...)
				ok = true
				continue
			}

			// ここからor条件
			for _, peer := range rule.To {
				// ここからand条件

				// NamespaceSelectorのチェック
				if peer.NamespaceSelector == nil {
					// Network Policyが属するNamespaceに属するPodが対象になる
					if policy.Namespace != destPod.Namespace {
						continue
					}
				} else {
					// 通信を受け入れるnamespaceが指定されている場合
					// srcPodが属しているnamespaceがpeer.NamespaceSelectorにマッチしているかチェックする
					if !isIncludedInLabelSelector(destPodNamespace.Labels, peer.NamespaceSelector) {
						continue
					}
				}

				// PodSelectorのチェック
				if !isIncludedInLabelSelector(destPod.Labels, peer.PodSelector) {
					continue
				}

				// IPBlockのチェック
				isIncluded, err := isIncludedInIpBlock(peer.IPBlock, destPod.Status.PodIP)
				if err != nil {
					return nil, false, err
				}
				if !isIncluded {
					continue
				}

				// ここまできたらdestPodはegressに含まれる
				egressPorts = append(egressPorts, rule.Ports...)
				ok = true
			}
		}
	}

	return egressPorts, ok, nil
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

// 別の処理にする
func isIncludedInLabelSelector(labels map[string]string, podSelector *metav1.LabelSelector) bool {
	if podSelector == nil {
		return true
	}

	// matchLabelとmatchExpressionは全てAND条件である。　一つでも条件を満たさなかったらfalseを返す。
	// matchLabel(等価ベース map)について
	if podSelector.MatchLabels != nil {
		for k, v := range podSelector.MatchLabels {
			if labels[k] != v {
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

func checkProtocol(targetProtocol string, otherProtocol string) (string, bool) {
	if targetProtocol == "any" && otherProtocol == "any" {
		return "any", true
	}

	if targetProtocol == "any" && otherProtocol != "any" {
		return otherProtocol, true
	}

	if targetProtocol != "any" && otherProtocol == "any" {
		return targetProtocol, true
	}

	if targetProtocol == otherProtocol {
		return targetProtocol, true
	}

	return "any", false
}

func checkPort(targetPort model.PortRealInfo, otherPort model.PortRealInfo) (int, int, bool) {
	if targetPort.Port == 0 && otherPort.Port == 0 {
		return 0, 0, true
	}

	if targetPort.Port == 0 && otherPort.Port != 0 {
		return otherPort.Port, otherPort.EndPort, true
	}

	if targetPort.Port != 0 && otherPort.Port == 0 {
		return targetPort.Port, targetPort.EndPort, true
	}

	// EndPort絡み
	if targetPort.EndPort == 0 && otherPort.EndPort == 0 {
		if targetPort.Port == otherPort.Port {
			return targetPort.Port, targetPort.EndPort, true
		}
		return 0, 0, false
	}

	if targetPort.EndPort == 0 && otherPort.EndPort != 0 {
		if otherPort.Port <= targetPort.Port && targetPort.Port <= otherPort.EndPort {
			return targetPort.Port, 0, true
		}
		return 0, 0, false
	}

	if targetPort.EndPort != 0 && otherPort.EndPort == 0 {
		if targetPort.Port <= otherPort.Port && otherPort.Port <= targetPort.EndPort {
			return otherPort.Port, 0, true
		}
		return 0, 0, false
	}

	// どっちもendあり
	if otherPort.Port < targetPort.Port {
		if targetPort.Port <= otherPort.EndPort && otherPort.EndPort <= targetPort.EndPort {
			return targetPort.Port, otherPort.EndPort, true
		}

		if targetPort.EndPort <= otherPort.EndPort {
			return targetPort.Port, targetPort.EndPort, true
		}

		return 0, 0, false
	} else {
		if otherPort.EndPort <= targetPort.EndPort {
			return otherPort.Port, otherPort.EndPort, true
		}

		if targetPort.EndPort <= otherPort.EndPort {
			return otherPort.Port, targetPort.EndPort, true
		}

		return 0, 0, false
	}
}

func getAccessPorts(targetPorts []model.PortRealInfo, otherPorts []model.PortRealInfo) ([]model.PortRealInfo, bool) {
	if len(targetPorts) == 0 && len(otherPorts) == 0 {
		return []model.PortRealInfo{}, true
	}

	if len(targetPorts) == 0 && len(otherPorts) != 0 {
		return otherPorts, true
	}

	if len(targetPorts) != 0 && len(otherPorts) == 0 {
		return targetPorts, true
	}

	res := make([]model.PortRealInfo, 0, len(targetPorts)*len(otherPorts))
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

			res = append(res, model.PortRealInfo{
				Protocol: protocol,
				Port:     port,
				EndPort:  endPort,
			})
		}
	}

	return res, len(res) != 0
}

// 重複しているPolicyPortがあれば削除する
func deleteDuplicationPolicyPort(l []netv1.NetworkPolicyPort) []model.PortRealInfo {
	res := make([]model.PortRealInfo, 0, len(l))

	if len(l) == 0 {
		return res
	}

	// nullableは扱いにくいので一旦stringかintに直す.
	m1 := make(map[string]map[int]map[int]model.PortRealInfo)
	for _, pp := range l {
		portInfo := model.Cast2PortRealInfo(pp)
		if _, ok := m1[portInfo.Protocol]; !ok {
			m3 := make(map[int]model.PortRealInfo)
			m2 := make(map[int]map[int]model.PortRealInfo)
			m3[portInfo.EndPort] = portInfo
			m2[portInfo.Port] = m3
			m1[portInfo.Protocol] = m2
		} else {
			// protocol自体は存在している
			if _, ok2 := m1[portInfo.Protocol][portInfo.Port]; !ok2 {
				m3 := make(map[int]model.PortRealInfo)
				m3[portInfo.EndPort] = portInfo
				m1[portInfo.Protocol][portInfo.Port] = m3
			} else {
				// protocol, port自体は存在している
				if _, ok3 := m1[portInfo.Protocol][portInfo.Port][portInfo.EndPort]; !ok3 {
					m1[portInfo.Protocol][portInfo.Port][portInfo.EndPort] = portInfo
				}
			}
		}
	}

	for _, m2 := range m1 {
		for _, m3 := range m2 {
			for _, portInfo := range m3 {
				res = append(res, portInfo)
			}
		}
	}

	return res
}
