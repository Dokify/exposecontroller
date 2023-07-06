package exposestrategy

import (
	"context"
	"fmt"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"net"
	"strconv"

	"github.com/pkg/errors"

	api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	client "k8s.io/client-go/kubernetes"
)

type NodePortStrategy struct {
	client  *client.Clientset
	encoder runtime.Encoder

	nodeIP string
}

var _ ExposeStrategy = &NodePortStrategy{}

const ExternalIPLabel = "fabric8.io/externalIP"

func NewNodePortStrategy(client *client.Clientset, encoder runtime.Encoder, nodeIP string) (*NodePortStrategy, error) {
	ip := nodeIP
	if len(ip) == 0 {
		l, err := client.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "failed to list nodes")
		}

		if len(l.Items) != 1 {
			return nil, errors.Errorf("node port strategy can only be used with single node clusters - found %d nodes", len(l.Items))
		}

		n := l.Items[0]
		ip = n.ObjectMeta.Annotations[ExternalIPLabel]
		if len(ip) == 0 {
			addr, err := getNodeHostIP(n)
			if err != nil {
				return nil, errors.Wrap(err, "cannot discover node IP")
			}
			ip = addr.String()
		}
	}

	return &NodePortStrategy{
		client:  client,
		nodeIP:  ip,
		encoder: encoder,
	}, nil
}

// getNodeHostIP returns the provided node's IP, based on the priority:
// 1. NodeExternalIP
// 2. NodeLegacyHostIP
// 3. NodeInternalIP
func getNodeHostIP(node api.Node) (net.IP, error) {
	addresses := node.Status.Addresses
	addressMap := make(map[api.NodeAddressType][]api.NodeAddress)
	for i := range addresses {
		addressMap[addresses[i].Type] = append(addressMap[addresses[i].Type], addresses[i])
	}
	if addresses, ok := addressMap[api.NodeExternalIP]; ok {
		return net.ParseIP(addresses[0].Address), nil
	}
	if addresses, ok := addressMap[api.NodeInternalIP]; ok {
		return net.ParseIP(addresses[0].Address), nil
	}
	return nil, fmt.Errorf("host IP unknown; known addresses: %v", addresses)
}

func (s *NodePortStrategy) Add(svc *api.Service) error {
	var err error
	clone := svc.DeepCopy()

	clone.Spec.Type = api.ServiceTypeNodePort

	if len(svc.Spec.Ports) == 0 {
		return errors.Errorf(
			"service %s/%s has no ports specified. Node port strategy requires a node port",
			svc.Namespace, svc.Name,
		)
	}

	if len(svc.Spec.Ports) > 1 {
		return errors.Errorf(
			"service %s/%s has multiple ports specified (%v). Node port strategy can only be used with single port services",
			svc.Namespace, svc.Name, svc.Spec.Ports,
		)
	}

	port := svc.Spec.Ports[0]
	portInt := int(port.NodePort)
	nodePort := strconv.Itoa(portInt)
	hostName := net.JoinHostPort(s.nodeIP, nodePort)
	if portInt > 0 {
		clone, err = addServiceAnnotation(clone, hostName)
	}
	clone.Spec.ExternalIPs = nil
	if err != nil {
		return errors.Wrap(err, "failed to add service annotation")
	}
	patch, err := createPatch(svc, clone, s.encoder, v1.Service{})
	if err != nil {
		return errors.Wrap(err, "failed to create patch")
	}
	if patch != nil {
		_, err = s.client.CoreV1().Services(svc.Namespace).Patch(context.Background(), svc.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to send patch for %s/%s patch %s", svc.Namespace, svc.Name, string(patch)))
		}
	}

	return nil
}

func (s *NodePortStrategy) Remove(svc *api.Service) error {
	clone := svc.DeepCopy()

	clone = removeServiceAnnotation(clone)

	patch, err := createPatch(svc, clone, s.encoder, v1.Service{})
	if err != nil {
		return errors.Wrap(err, "failed to create patch")
	}
	if patch != nil {
		_, err = s.client.CoreV1().Services(clone.Namespace).Patch(context.Background(), clone.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
		if err != nil {
			return errors.Wrap(err, "failed to send patch")
		}
	}

	return nil
}
