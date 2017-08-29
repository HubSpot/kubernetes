package requireresources

import (
	"errors"
	"io"

	"k8s.io/apiserver/pkg/admission"
	"k8s.io/kubernetes/pkg/api"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"fmt"
)

func Register(plugins *admission.Plugins) {
	plugins.Register("RequireResources", func(config io.Reader) (admission.Interface, error) {
		return NewRequireResources(), nil
	})
}

type requireResources struct{}

func isEmptyResources(resources *api.ResourceRequirements) bool {
	return len(resources.Requests) == 0 && len(resources.Limits) == 0
}

func (requireResources) Admit(attributes admission.Attributes) (err error) {
	if len(attributes.GetSubresource()) != 0 || attributes.GetResource().GroupResource() != api.Resource("pods") {
		return nil
	}
	pod, ok := attributes.GetObject().(*api.Pod)
	if !ok {
		return apierrors.NewBadRequest("Resource was marked with kind Pod but was unable to be converted")
	}

	if _, ok := pod.Annotations["hubspot.com/allow-best-effort"]; ok {
		return nil
	}

	for i := range pod.Spec.InitContainers {
		if isEmptyResources(&pod.Spec.InitContainers[i].Resources) {
			return apierrors.NewBadRequest(fmt.Sprintf("Init container '%s' must have resources set.", pod.Spec.InitContainers[i].Name))
		}
	}

	for i := range pod.Spec.Containers {
		if isEmptyResources(&pod.Spec.Containers[i].Resources) {
			return apierrors.NewBadRequest(fmt.Sprintf("Container '%s' must have resources set.", pod.Spec.Containers[i].Name))
		}
	}

	return nil
}

func (requireResources) Handles(operation admission.Operation) bool {
	return true
}

func NewRequireResources() admission.Interface {
	return new(requireResources)
}

