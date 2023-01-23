package model

import v1 "k8s.io/api/core/v1"

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
	Ports     []PortInfo `json:"ports"`
}

type PortInfo struct {
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	EndPort  int    `json:"end_port"`
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
		return "unknown"
	}
}
