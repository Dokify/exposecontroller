package exposestrategy

import (
	"context"
	"strings"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	client "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

const (
	ingress            = "ingress"
	loadBalancer       = "loadbalancer"
	nodePort           = "nodeport"
	route              = "route"
	domainExt          = ".nip.io"
	stackpointNS       = "stackpoint-system"
	stackpointHAProxy  = "spc-balancer"
	stackpointIPEnvVar = "BALANCER_IP"
)

func NewAutoStrategy(exposer, domain, internalDomain, urltemplate string, nodeIP, routeHost, pathMode string, routeUsePath, http, tlsAcme bool, tlsSecretName string, tlsUseWildcard bool, ingressClass string, client *client.Clientset, restClientConfig *restclient.Config, encoder runtime.Encoder) (ExposeStrategy, error) {

	exposer, err := getAutoDefaultExposeRule(client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to automatically get exposer rule.  consider setting 'exposer' type in config.yml")
	}
	glog.Infof("Using exposer strategy: %s", exposer)

	// only try to get domain if we need wildcard dns and one wasn't given to us
	if len(domain) == 0 && (strings.EqualFold(ingress, exposer)) {
		domain, err = getAutoDefaultDomain(client)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get a domain")
		}
		glog.Infof("Using domain: %s", domain)
	}

	return New(exposer, domain, internalDomain, urltemplate, nodeIP, routeHost, pathMode, routeUsePath, http, tlsAcme, tlsSecretName, tlsUseWildcard, ingressClass, client, restClientConfig, encoder)
}

func getAutoDefaultExposeRule(c *client.Clientset) (string, error) {
	t, err := typeOfMaster(c)
	if err != nil {
		return "", errors.Wrap(err, "failed to get type of master")
	}
	if t == openShift {
		return route, nil
	}

	// lets default to Ingress on kubernetes for now
	/*
		nodes, err := c.CoreV1().Nodes().List(metav1.ListOption{})
		if err != nil {
			return "", errors.Wrap(err, "failed to find any nodes")
		}
		if len(nodes.Items) == 1 {
			node := nodes.Items[0]
			if node.Name == "minishift" || node.Name == "minikube" {
				return nodePort, nil
			}
		}
	*/
	return ingress, nil
}

func getAutoDefaultDomain(c *client.Clientset) (string, error) {
	ctx := context.Background()
	nodes, err := c.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", errors.Wrap(err, "failed to find any nodes")
	}

	// if we're mini* then there's only one node, any router / ingress controller deployed has to be on this one
	if len(nodes.Items) == 1 {
		node := nodes.Items[0]
		if node.Name == "minishift" || node.Name == "minikube" {
			ip, err := getExternalIP(node)
			if err != nil {
				return "", err
			}
			return ip + domainExt, nil
		}
	}

	// check for a gofabric8 ingress labelled node
	//selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: map[string]string{"fabric8.io/externalIP": "true"}})
	/*selector, err := metav1.ParseToLabelSelector("fabric8.io/externalIP=true")
	if err != nil {
		log.Fatalf("Error parsing selector: %v", err)
	}

	selectorAsSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		log.Fatalf("Error building selector: %v", err)
	}*/

	selector := "fabric8.io/externalIP=true"
	nodes, err = c.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: selector})
	if len(nodes.Items) == 1 {
		node := nodes.Items[0]
		ip, err := getExternalIP(node)
		if err != nil {
			return "", err
		}
		return ip + domainExt, nil
	}

	// look for a stackpoint HA proxy
	pod, _ := c.CoreV1().Pods(stackpointNS).Get(ctx, stackpointHAProxy, metav1.GetOptions{})
	if pod != nil {
		containers := pod.Spec.Containers
		for _, container := range containers {
			if container.Name == stackpointHAProxy {
				for _, e := range container.Env {
					if e.Name == stackpointIPEnvVar {
						return e.Value + domainExt, nil
					}
				}
			}
		}
	}
	return "", errors.New("no known automatic ways to get an external ip to use with nip.  Please configure exposecontroller configmap manually see https://github.com/jenkins-x/exposecontroller#configuration")
}

// copied from k8s.io/kubernetes/pkg/master/master.go
func getExternalIP(node api.Node) (string, error) {
	var fallback string
	ann := node.Annotations
	if ann != nil {
		for k, v := range ann {
			if len(v) > 0 && strings.HasSuffix(k, "kubernetes.io/provided-node-ip") {
				return v, nil
			}
		}
	}
	for ix := range node.Status.Addresses {
		addr := &node.Status.Addresses[ix]
		if addr.Type == api.NodeExternalIP {
			return addr.Address, nil
		}
		if fallback == "" && addr.Type == api.NodeInternalIP {
			fallback = addr.Address
		}
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", errors.New("no node ExternalIP or LegacyHostIP found")
}
