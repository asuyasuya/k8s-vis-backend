package main

import (
	"github.com/asuyasuya/k8s-vis-backend/src/config"
	"github.com/asuyasuya/k8s-vis-backend/src/controller"
)

func main() {
	clientset, err := config.NewClient()
	if err != nil {
		panic(err.Error())
	}
	ctrl := controller.NewController(clientset)
	router := config.GetRouter(ctrl)
	router.Run(":8080")
}
