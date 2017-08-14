package ssfixup

import (
	"k8s.io/apiserver/pkg/admission"
	"io"
	"k8s.io/kubernetes/pkg/api"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	podapi "k8s.io/kubernetes/pkg/api/v1/pod"
)

func init() {
	admission.RegisterPlugin("SSFixup", func(config io.Reader) (admission.Interface, error) {
		return NewSSFixup(), nil
	})
}

// alwaysPullImages is an implementation of admission.Interface.
// It looks at all new pods and overrides each container's image pull policy to Always.
type ssFixup struct {
	*admission.Handler
}

func (ssf *ssFixup) Admit(attributes admission.Attributes) (err error) {
	// Ignore all calls to subresources or resources other than pods.
	if len(attributes.GetSubresource()) != 0 || attributes.GetResource().GroupResource() != api.Resource("pods") {
		return nil
	}
	pod, ok := attributes.GetObject().(*api.Pod)
	if !ok {
		return apierrors.NewBadRequest("Resource was marked with kind Pod but was unable to be converted")
	}

	if value, ok := pod.Annotations[podapi.PodHostnameAnnotation]; ok {
		pod.Spec.Hostname = value
	}

	if value, ok := pod.Annotations[podapi.PodSubdomainAnnotation]; ok {
		pod.Spec.Subdomain = value
	}

	return nil
}

func NewSSFixup() admission.Interface {
	return &ssFixup{
		Handler: admission.NewHandler(admission.Create, admission.Update),
	}
}
