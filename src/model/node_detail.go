package model

type NodeDetailViewModel struct {
	Name    string `json:"name"`
	Ip      string `json:"ip"`
	PodCidr string `json:"pod_cidr"`
}
