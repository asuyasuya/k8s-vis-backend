package controller

import (
	"k8s.io/client-go/kubernetes"
)

type Ctrl struct {
	kubeClient *kubernetes.Clientset
}

func NewController(kubeClient *kubernetes.Clientset) *Ctrl {
	return &Ctrl{
		kubeClient: kubeClient,
	}
}
