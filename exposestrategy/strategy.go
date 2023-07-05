package exposestrategy

import (
	"strings"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	"k8s.io/apimachinery/pkg/runtime"
	client "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

type ExposeStrategy interface {
	Add(svc *v1.Service) error
	Remove(svc *v1.Service) error
}

type Label struct {
	Key   string
	Value string
}

var (
	ExposeLabel                   = Label{Key: "expose", Value: "true"}
	ExposeAnnotation              = Label{Key: "fabric8.io/expose", Value: "true"}
	InjectAnnotation              = Label{Key: "fabric8.io/inject", Value: "true"}
	ExposeHostNameAsAnnotationKey = "fabric8.io/exposeHostNameAs"
	ExposeAnnotationKey           = "fabric8.io/exposeUrl"
	ExposePortAnnotationKey       = "fabric8.io/exposePort"
	ApiServicePathAnnotationKey   = "v1.Service.kubernetes.io/path"
)

func New(exposer, domain, internalDomain, urltemplate, nodeIP, routeHost, pathMode string, routeUsePath, http, tlsAcme bool, tlsSecretName string, tlsUseWildcard bool, ingressClass string, client *client.Clientset, restClientConfig *restclient.Config, encoder runtime.Encoder) (ExposeStrategy, error) {
	switch strings.ToLower(exposer) {
	case "ambassador":
		strategy, err := NewAmbassadorStrategy(client, encoder, domain, http, tlsAcme, tlsSecretName, urltemplate, pathMode)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create ambassador expose strategy")
		}
		return strategy, nil
	case "loadbalancer":
		strategy, err := NewLoadBalancerStrategy(client, encoder)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create load balancer expose strategy")
		}
		return strategy, nil
	case "nodeport":
		strategy, err := NewNodePortStrategy(client, encoder, nodeIP)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create node port expose strategy")
		}
		return strategy, nil
	case "ingress":
		glog.Infof("stratagy.New %v", http)
		strategy, err := NewIngressStrategy(client, encoder, domain, internalDomain, http, tlsAcme, tlsSecretName, tlsUseWildcard, urltemplate, pathMode, ingressClass)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create ingress expose strategy")
		}
		return strategy, nil
	case "route":
		ocfg := *restClientConfig
		ocfg.APIPath = ""
		ocfg.GroupVersion = nil
		ocfg.NegotiatedSerializer = nil
		oc, _ := oclient.New(&ocfg)
		strategy, err := NewRouteStrategy(client, oc, encoder, domain, routeHost, routeUsePath, http)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create ingress expose strategy")
		}
		return strategy, nil
	case "":
		strategy, err := NewAutoStrategy(exposer, domain, internalDomain, urltemplate, nodeIP, routeHost, pathMode, routeUsePath, http, tlsAcme, tlsSecretName, tlsUseWildcard, ingressClass, client, restClientConfig, encoder)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create auto expose strategy")
		}
		return strategy, nil
	default:
		return nil, errors.Errorf("unknown expose strategy '%s', must be one of %v", exposer, []string{"Auto", "Ingress", "Route", "NodePort", "LoadBalancer"})
	}
}
