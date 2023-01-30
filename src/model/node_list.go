package model

type PodViewModel struct {
	Name string `json:"name"`
}

type NodeViewModel struct {
	Name     string         `json:"name"`
	TotalPod int            `json:"total_pod"`
	Pods     []PodViewModel `json:"pods"`
}

type NodeListViewModel struct {
	TotalNode int             `json:"total_node"`
	Nodes     []NodeViewModel `json:"nodes"`
}
