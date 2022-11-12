package config

import (
	"flag"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"path/filepath"
)

func NewClient() (*kubernetes.Clientset, error) {
	var kubeconfig *string
	kubeconfig = flag.String("kubeconfig", filepath.Join("/", "go", "src", "app", ".kube", "config"), "absolute path to the kubeconfig file")
	flag.Parse()
	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		return nil, err
	}

	// create the clientset
	return kubernetes.NewForConfig(config)
}
