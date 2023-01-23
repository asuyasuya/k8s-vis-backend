package controller

import (
	"context"
	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"log"
	"net/http"
)

type Ctrl struct {
	kubeClient *kubernetes.Clientset
}

func NewController(kubeClient *kubernetes.Clientset) *Ctrl {
	return &Ctrl{
		kubeClient: kubeClient,
	}
}

func (c *Ctrl) GetTest() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		policy, err := c.kubeClient.NetworkingV1().NetworkPolicies("default").List(context.TODO(), metav1.ListOptions{
			FieldSelector: "metadata.name=test-policy-empty2"})
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		log.Println("policy types = ", policy.Items[0].Spec.PolicyTypes)
		log.Println("has ingress = ", hasIngress(policy.Items[0].Spec.PolicyTypes))
		log.Println("has egress = ", hasEgress(policy.Items[0].Spec.PolicyTypes))
		for _, v := range policy.Items {
			log.Println("++++++++++")
			log.Println("ingress rule length = ", len(v.Spec.Ingress))
			log.Println(v.Spec) // {{map[app:app4] []} [{[] []}] [] [Ingress]} {{map[app:app5] []} [{[] [{&LabelSelector{MatchLabels:map[string]string{},MatchExpressions:[]LabelSelectorRequirement{},} nil nil}]}] [] [Ingress]}

			for _, rule := range v.Spec.Ingress {
				log.Println("======")
				log.Println("ingress peer length = ", len(rule.From))
				for _, peer := range rule.From {
					log.Println("namespaceSelector = ", peer.NamespaceSelector)
					log.Println("namespaceSelector is null = ", peer.NamespaceSelector == nil)
					log.Println("podSelector = ", peer.PodSelector)
					log.Println("ipBlock = ", peer.IPBlock)
					log.Println("---")
				}
				log.Println("ports", rule.Ports)
				log.Println("======")
			}
			log.Println("++++++++++")

		}
		ctx.JSON(200, gin.H{
			"message": "OK",
		})
	}
}
