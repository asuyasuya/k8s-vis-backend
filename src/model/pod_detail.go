package model

import (
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type PodDetailViewModel struct {
	Name        string      `json:"name"`
	Ip          string      `json:"ip"`
	Namespace   string      `json:"namespace"`
	Labels      []Label     `json:"labels"`
	AccessPods  []AccessPod `json:"access_pods"`
	PolicyNames []string    `json:"policy_names"`
}

type Label struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func LabelViewModel(pod v1.Pod) []Label {
	labels := make([]Label, 0, len(pod.Labels))
	for k, v := range pod.Labels {
		labels = append(labels, Label{
			Key:   k,
			Value: v,
		})
	}
	return labels
}

type AccessPod struct {
	Name      string    `json:"name"`
	Ip        string    `json:"ip"`
	Namespace string    `json:"namespace"`
	Labels    []Label   `json:"labels"`
	Ingress   PodPolicy `json:"ingress"`
	Egress    PodPolicy `json:"egress"`
}

type PodPolicy struct {
	CanAccess bool       `json:"can_access"`
	Ports     []PortInfo `json:"ports,omitempty"`
}

type PortRealInfo struct {
	Protocol string
	Port     int
	EndPort  int
}

type PortInfo struct {
	Protocol *interface{} `json:"protocol,omitempty"`
	Port     *interface{} `json:"port,omitempty"`
	EndPort  *interface{} `json:"end_port,omitempty"`
}

func Cast2PortRealInfo(port netv1.NetworkPolicyPort) PortRealInfo {
	return PortRealInfo{
		Protocol: CastProtocol(port.Protocol),
		Port:     CastPort(port.Port),
		EndPort:  CastEndPort(port.EndPort),
	}
}

func CastProtocol(protocol *v1.Protocol) string {
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
		return "any"
	}
}

func CastPort(port *intstr.IntOrString) int {
	if port == nil {
		return 0
	}

	return int(port.IntVal)
}

func CastEndPort(endPort *int32) int {
	if endPort == nil {
		return 0
	}

	return int(*endPort)
}

func Cast2PortInfo(info PortRealInfo) PortInfo {
	var res PortInfo

	// protocol
	switch info.Protocol {
	case "TCP", "UDP", "SCTP":
		var protocol interface{}
		protocol = info.Protocol
		res.Protocol = &protocol
	}

	// port
	if info.Port != 0 {
		var port interface{}
		port = info.Port
		res.Port = &port
	}

	// port
	if info.EndPort != 0 {
		var endPort interface{}
		endPort = info.EndPort
		res.EndPort = &endPort
	}

	return res
}
