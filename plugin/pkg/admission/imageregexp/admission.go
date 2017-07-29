package imageregexp

import (
	"io"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/kubernetes/pkg/api"

	"k8s.io/apimachinery/pkg/util/yaml"
	"regexp"

	"github.com/golang/glog"
)

func init() {
	admission.RegisterPlugin("ImageRegexp", func(config io.Reader) (admission.Interface, error) {
		newImageRegexp, err := NewImageRegexp(config)
		if err != nil {
			return nil, err
		}
		return newImageRegexp, nil
	})
}

type imageRegexReplacement struct {
	Regexp      *regexp.Regexp
	Replacement string
}

type imageRegexp struct {
	*admission.Handler
	Items []imageRegexReplacement
}

func (ir *imageRegexp) handleContainer(container *api.Container) {
	for i := range ir.Items {
		container.Image = ir.Items[i].Regexp.ReplaceAllString(container.Image, ir.Items[i].Replacement)
	}
}

func (ir *imageRegexp) Admit(attributes admission.Attributes) (err error) {
	// bail early if no replacements
	if len(ir.Items) == 0 {
		return nil
	}

	// Ignore all calls to subresources or resources other than pods.
	if len(attributes.GetSubresource()) != 0 || attributes.GetResource().GroupResource() != api.Resource("pods") {
		return nil
	}
	pod, ok := attributes.GetObject().(*api.Pod)
	if !ok {
		return apierrors.NewBadRequest("Resource was marked with kind Pod but was unable to be converted")
	}

	for i := range pod.Spec.InitContainers {
		ir.handleContainer(&pod.Spec.InitContainers[i])
	}

	for i := range pod.Spec.Containers {
		ir.handleContainer(&pod.Spec.Containers[i])
	}

	return nil
}

func NewImageRegexp(config io.Reader) (admission.Interface, error) {
	var ac AdmissionConfig
	d := yaml.NewYAMLOrJSONDecoder(config, 4096)
	err := d.Decode(&ac)
	if err != nil {
		return nil, err
	}

	items := make([]imageRegexReplacement, len(ac.ImageRegexpConfigs))

	// compile the regexp(s), bail early if compilation fails
	for i := range ac.ImageRegexpConfigs {
		configItem := ac.ImageRegexpConfigs[i]

		regexp, err := regexp.Compile(configItem.Regexp)

		if err != nil {
			return nil, err
		}

		glog.V(2).Infof("Compiled ImageRegexpConfig %s", configItem)

		items[i] = imageRegexReplacement{
			Regexp: regexp,
			Replacement: configItem.Replacement,
		}
	}

	return &imageRegexp{
		Handler: admission.NewHandler(admission.Create, admission.Update),
		Items:   items,
	}, nil
}
