package imageregexp

import (
	"io"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/admission"

	"k8s.io/apimachinery/pkg/util/yaml"
	"regexp"

	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/apis/extensions/v1beta1"
	"net/http"
)

var DockerImageRegex = regexp.MustCompile("^(.*?)/(.*?):([^:]+)$")

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
	Regexp           *regexp.Regexp
	Replacement      string
	ResolveDockerTag bool
}

type imageRegexp struct {
	*admission.Handler
	Items []imageRegexReplacement
}

type dockerConfig struct {
	Digest string `json:digest`
}

type dockerManifest struct {
	Config dockerConfig `json:config`
}

func resolveDockerTag(registryHost string, imageName string, tagName string) (string, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", registryHost, imageName, tagName)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("Error requesting manfiest (%s): %s", url, err)
	}

	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Error requesting manfiest (%s): %s", url, err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("Error requesting manfiest (%s): %s", url, err)
	}

	deserialized := new(dockerManifest)
	if err := json.Unmarshal(body, deserialized); err != nil {
		return "", fmt.Errorf("Error requesting manifest (%s): %s", url, err)
	}

	return deserialized.Config.Digest, nil
}

func (ir *imageRegexp) handleContainer(container *v1.Container) error {
	for _, irr := range ir.Items {
		if len(irr.Replacement) > 0 {
			container.Image = irr.Regexp.ReplaceAllString(container.Image, irr.Replacement)
		}

		if irr.ResolveDockerTag {
			matches := DockerImageRegex.FindStringSubmatch(container.Image)

			if matches == nil {
				return fmt.Errorf("Docker image name regexp failed for '%s'", container.Image)
			}

			registryHost, imageName, tagName := matches[0], matches[1], matches[2]

			resolvedTag, err := resolveDockerTag(registryHost, imageName, tagName)

			if err != nil {
				return fmt.Errorf("Failed to resolve docker tag for %s: %s", container.Image, err)
			}

			glog.V(2).Infof("Resolved %s to Docker tag %s", container.Image, resolvedTag)

			container.Image = fmt.Sprintf("%s%s:%s", registryHost, imageName, resolvedTag)
		}
	}

	return nil
}

func (ir *imageRegexp) handlePodSpec(podSpec *v1.PodSpec) error {
	for _, initContainer := range podSpec.InitContainers {
		if err := ir.handleContainer(&initContainer); err != nil {
			return fmt.Errorf("Error handling InitContainer '%s': %s", initContainer.Name, err)
		}
	}

	for _, container := range podSpec.Containers {
		if err := ir.handleContainer(&container); err != nil {
			return fmt.Errorf("Error handling Container '%s': %s", container.Name, err)
		}
	}

	return nil
}

func buildBadRequestUnableToConvert(obj runtime.Object) error {
	return apierrors.NewBadRequest(fmt.Sprintf("Resource was marked with kind '%s' but was unable to be converted", obj.GetObjectKind().GroupVersionKind().Kind))
}

func buildKindError(obj runtime.Object, name string, err error) error {
	return fmt.Errorf("Error handling %s '%s': %s", obj.GetObjectKind().GroupVersionKind().Kind, name, err)
}

func (ir *imageRegexp) Admit(attributes admission.Attributes) (err error) {
	// bail early if no replacements
	if len(ir.Items) == 0 {
		return nil
	}

	switch attributes.GetResource().GroupResource().Resource {
	case "pods":
		pod, ok := attributes.GetObject().(*v1.Pod)
		if !ok {
			return buildBadRequestUnableToConvert(attributes.GetObject())
		}
		if err := ir.handlePodSpec(&pod.Spec); err != nil {
			return buildKindError(pod, pod.Name, err)
		}
	case "replicasets":
		rs, ok := attributes.GetObject().(v1beta1.ReplicaSet)
		if !ok {
			return buildBadRequestUnableToConvert(attributes.GetObject())
		}
		if err := ir.handlePodSpec(&rs.Spec.Template.Spec); err != nil {
			return buildKindError(rs, rs.Name, err)
		}
	case "deployments":
		d, ok := attributes.GetObject().(v1beta1.Deployment)
		if !ok {
			return buildBadRequestUnableToConvert(attributes.GetObject())
		}
		if err := ir.handlePodSpec(&d.Spec.Template.Spec); err != nil {
			return buildKindError(d, d.Name, err)
		}
	case "daemonsets":
		ds, ok := attributes.GetObject().(*v1beta1.DaemonSet)
		if !ok {
			return buildBadRequestUnableToConvert(attributes.GetObject())
		}
		if err := ir.handlePodSpec(&ds.Spec.Template.Spec); err != nil {
			return buildKindError(ds, ds.Name, err)
		}
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
	for i, configItem := range ac.ImageRegexpConfigs {
		regexp, err := regexp.Compile(configItem.Regexp)

		if err != nil {
			return nil, err
		}

		glog.V(2).Infof("Compiled ImageRegexpConfig %s", configItem)

		items[i] = imageRegexReplacement{
			Regexp:      regexp,
			Replacement: configItem.Replacement,
		}
	}

	return &imageRegexp{
		Handler: admission.NewHandler(admission.Create, admission.Update),
		Items:   items,
	}, nil
}
