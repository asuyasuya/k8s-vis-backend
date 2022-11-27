package model

import v1 "k8s.io/api/core/v1"

type PortInfo struct {
	protocol string
	port     int
}

type PolicyInfo struct {
	ports     []PortInfo
	canAccess bool
}

type PodWithPolicy struct {
	pod       v1.Pod
	ingress   PolicyInfo
	egress    PolicyInfo
	targetPod v1.Pod
}
