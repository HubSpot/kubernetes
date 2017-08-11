package imageregexp

import (
	"io"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/kubernetes/pkg/api"

	"k8s.io/apimachinery/pkg/util/yaml"
	"regexp"

	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"io/ioutil"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"net/http"
)

var DockerImageRegex = regexp.MustCompile("^(.*?)/(.*?):([^:]+)$")

func Register(plugins *admission.Plugins) {
	plugins.Register("ImageRegexp", func(config io.Reader) (admission.Interface, error) {
		newImageRegexp, err := NewImageRegexp(config)
		if err != nil {
			return nil, err
		}
		return newImageRegexp, nil
	})
}

type imageRegexReplacement struct {
	CompiledRegexp *regexp.Regexp
	Config         *imageRegexpConfig
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
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registryHost, imageName, tagName)

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

func (ir *imageRegexp) handleContainer(container *api.Container) error {
	for _, irr := range ir.Items {
		if irr.CompiledRegexp.MatchString(container.Image) {
			if len(irr.Config.Replacement) > 0 {
				newImage := irr.CompiledRegexp.ReplaceAllString(container.Image, irr.Config.Replacement)
				glog.V(2).Infof("Updated image from '%s' to '%s'", container.Image, newImage)
				container.Image = newImage
			}

			if irr.Config.ResolveTag {
				matches := DockerImageRegex.FindStringSubmatch(container.Image)

				if matches == nil {
					return fmt.Errorf("Docker image name regexp failed for '%s'", container.Image)
				}

				registryHost, imageName, tagName := matches[1], matches[2], matches[3]

				resolvedTag, err := resolveDockerTag(registryHost, imageName, tagName)

				if err != nil {
					return fmt.Errorf("Failed to resolve docker tag for image '%s': %s", container.Image, err)
				}

				glog.V(2).Infof("Resolved image '%s' to Docker tag '%s'", container.Image, resolvedTag)

				container.Image = fmt.Sprintf("%s%s:%s", registryHost, imageName, resolvedTag)
			}
		}
	}

	return nil
}

func (ir *imageRegexp) handlePodSpec(podSpec *api.PodSpec) error {
	for i := range podSpec.InitContainers {
		initContainer := &podSpec.InitContainers[i]
		if err := ir.handleContainer(initContainer); err != nil {
			return fmt.Errorf("Error handling InitContainer '%s': %s", initContainer.Name, err)
		}
	}

	for i := range podSpec.Containers {
		container := &podSpec.Containers[i]
		if err := ir.handleContainer(container); err != nil {
			return fmt.Errorf("Error handling Container '%s': %s", container.Name, err)
		}
	}

	return nil
}

func buildBadRequestUnableToConvert(attr admission.Attributes) error {
	glog.V(2).Infof("Resource type '%s' was unable to be converted: %s", attr.GetResource().Resource, attr.GetObject())
	return apierrors.NewBadRequest(fmt.Sprintf("Resource type '%s' was unable to be converted", attr.GetResource().Resource))
}

func buildKindError(name string, err error) error {
	return fmt.Errorf("Error handling '%s': %s", name, err)
}

func (ir *imageRegexp) Admit(attributes admission.Attributes) (err error) {
	// bail early if no replacements
	if len(ir.Items) == 0 {
		return nil
	}

	// dont mess with subresources
	if len(attributes.GetSubresource()) != 0 {
		return nil
	}

	switch attributes.GetResource().GroupResource().Resource {
	case "pods":
		pod, ok := attributes.GetObject().(*api.Pod)
		if !ok {
			return buildBadRequestUnableToConvert(attributes)
		}
		if err := ir.handlePodSpec(&pod.Spec); err != nil {
			return buildKindError(pod.Name, err)
		}
	case "replicasets":
		rs, ok := attributes.GetObject().(*extensions.ReplicaSet)
		if !ok {
			return buildBadRequestUnableToConvert(attributes)
		}
		if err := ir.handlePodSpec(&rs.Spec.Template.Spec); err != nil {
			return buildKindError(rs.Name, err)
		}
	case "deployments":
		d, ok := attributes.GetObject().(*extensions.Deployment)
		if !ok {
			return buildBadRequestUnableToConvert(attributes)
		}
		if err := ir.handlePodSpec(&d.Spec.Template.Spec); err != nil {
			return buildKindError(d.Name, err)
		}
	case "daemonsets":
		ds, ok := attributes.GetObject().(*extensions.DaemonSet)
		if !ok {
			return buildBadRequestUnableToConvert(attributes)
		}
		if err := ir.handlePodSpec(&ds.Spec.Template.Spec); err != nil {
			return buildKindError(ds.Name, err)
		}
	}

	return nil
}

func NewImageRegexp(config io.Reader) (admission.Interface, error) {
	var ac AdmissionConfig
	d := yaml.NewYAMLOrJSONDecoder(config, 4096)
	err := d.Decode(&ac)
	if err != nil {
		return nil, fmt.Errorf("Error decoding AdmissionConfig for ImageRegexp: %s", err)
	}

	items := make([]imageRegexReplacement, len(ac.ImageRegexpConfigs))

	// compile the regexp(s), bail early if compilation fails
	for i, configItem := range ac.ImageRegexpConfigs {
		regexp, err := regexp.Compile(configItem.Regexp)

		if err != nil {
			return nil, fmt.Errorf("Error compiling regexp for %s: %s", configItem, err)
		}

		glog.V(2).Infof("Compiled ImageRegexpConfig %s", configItem)

		items[i] = imageRegexReplacement{
			CompiledRegexp: regexp,
			Config:         &configItem,
		}
	}

	return &imageRegexp{
		Handler: admission.NewHandler(admission.Create, admission.Update),
		Items:   items,
	}, nil
}
