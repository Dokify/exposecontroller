package exposestrategy

import (
	"reflect"

	"github.com/pkg/errors"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	client "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"

	"k8s.io/apimachinery/pkg/runtime"
)

type LoadBalancerStrategy struct {
	client  *client.Client
	encoder runtime.Encoder
}

var _ ExposeStrategy = &LoadBalancerStrategy{}

func NewLoadBalancerStrategy(client *client.Client, encoder runtime.Encoder) (*LoadBalancerStrategy, error) {
	return &LoadBalancerStrategy{
		client:  client,
		encoder: encoder,
	}, nil
}

func (s *LoadBalancerStrategy) Add(svc *v1.Service) error {
	cloned, err := scheme.Scheme.DeepCopy(svc)
	if err != nil {
		return errors.Wrap(err, "failed to clone service")
	}
	clone, ok := cloned.(*v1.Service)
	if !ok {
		return errors.Errorf("cloned to wrong type: %s", reflect.TypeOf(cloned))
	}

	clone.Spec.Type = v1.ServiceTypeLoadBalancer
	if len(clone.Spec.LoadBalancerIP) > 0 {
		clone, err = addServiceAnnotation(clone, clone.Spec.LoadBalancerIP)
		if err != nil {
			return errors.Wrap(err, "failed to add service annotation")
		}
	}

	patch, err := createPatch(svc, clone, s.encoder, v1.Service{})
	if err != nil {
		return errors.Wrap(err, "failed to create patch")
	}
	if patch != nil {
		err = s.client.Patch(strategicpatch.StrategicMergePatchType).
			Resource("services").
			Namespace(svc.Namespace).
			Name(svc.Name).
			Body(patch).Do().Error()
		if err != nil {
			return errors.Wrap(err, "failed to send patch")
		}
	}

	return nil
}

func (s *LoadBalancerStrategy) Remove(svc *v1.Service) error {
	cloned, err := scheme.Scheme.DeepCopy(svc)
	if err != nil {
		return errors.Wrap(err, "failed to clone service")
	}
	clone, ok := cloned.(*v1.Service)
	if !ok {
		return errors.Errorf("cloned to wrong type: %s", reflect.TypeOf(svc))
	}

	clone = removeServiceAnnotation(clone)

	patch, err := createPatch(svc, clone, s.encoder, v1.Service{})
	if err != nil {
		return errors.Wrap(err, "failed to create patch")
	}
	if patch != nil {
		err = s.client.Patch(strategicpatch.StrategicMergePatchType).
			Resource("services").
			Namespace(clone.Namespace).
			Name(clone.Name).
			Body(patch).Do().Error()
		if err != nil {
			return errors.Wrap(err, "failed to send patch")
		}
	}

	return nil
}
