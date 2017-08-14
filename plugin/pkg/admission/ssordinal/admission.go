package ssordinal

import (
	"k8s.io/apiserver/pkg/admission"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"io"
	"k8s.io/kubernetes/pkg/api"
	"strings"
)

func Register(plugins *admission.Plugins) {
	plugins.Register("SSOrdinal", func(config io.Reader) (admission.Interface, error) {
		return NewSSOrdinal()
	})
}

type ssOrdinal struct {
	*admission.Handler
}

func (sso *ssOrdinal) Admit(attributes admission.Attributes) error {
	// Ignore all calls to subresources or resources other than pods.
	if len(attributes.GetSubresource()) != 0 || attributes.GetResource().GroupResource() != api.Resource("pods") {
		return nil
	}
	pod, ok := attributes.GetObject().(*api.Pod)
	if !ok {
		return apierrors.NewBadRequest("Resource was marked with kind Pod but was unable to be converted")
	}

	// If this pod is owned by a StatefulSet, set the ss-ordinal label
	for i := range pod.OwnerReferences {
		ownerRef := pod.OwnerReferences[i]

		if ownerRef.Kind == "StatefulSet" && strings.HasPrefix(pod.Name, ownerRef.Name) {
			pod.Labels["hubspot.com/ss-ordinal"] = pod.Name[len(ownerRef.Name)+1:]
		}
	}

	return nil
}

func NewSSOrdinal() (admission.Interface, error) {
	return &ssOrdinal{
		Handler: admission.NewHandler(admission.Create),
	}, nil
}