package ssfixup

import (
	"k8s.io/apiserver/pkg/admission"
	"io"
	"k8s.io/kubernetes/pkg/api"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	podapi "k8s.io/kubernetes/pkg/api/v1/pod"
	"github.com/golang/glog"
)

func init() {
	admission.RegisterPlugin("SSFixup", func(config io.Reader) (admission.Interface, error) {
		return NewSSFixup(), nil
	})
}

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

	glog.V(2).Infof("Got pod %s", pod.Name)

	glog.V(2).Infof("Annotation %s = %s", podapi.PodHostnameAnnotation, pod.Annotations[podapi.PodHostnameAnnotation])
	glog.V(2).Infof("Annotation %s = %s", podapi.PodSubdomainAnnotation, pod.Annotations[podapi.PodSubdomainAnnotation])
	glog.V(2).Infof("pod.Spec.Hostname = '%s'", pod.Spec.Hostname)
	glog.V(2).Infof("pod.Spec.Subdomain = '%s'", pod.Spec.Subdomain)

	if value, ok := pod.Annotations[podapi.PodHostnameAnnotation]; ok && pod.Spec.Hostname == "" {
		glog.V(2).Infof("Setting pod.Spec.Hostname to '%s'", value)
		pod.Spec.Hostname = value
	}

	if value, ok := pod.Annotations[podapi.PodSubdomainAnnotation]; ok && pod.Spec.Subdomain == "" {
		glog.V(2).Infof("Setting pod.Spec.Subdomain to '%s'", value)
		pod.Spec.Subdomain = value
	}

	return nil
}

func NewSSFixup() admission.Interface {
	return &ssFixup{
		Handler: admission.NewHandler(admission.Create, admission.Update),
	}
}
