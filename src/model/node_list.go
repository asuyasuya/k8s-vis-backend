package model

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
