package imageregexp

import (
	"testing"
	"strings"

	"k8s.io/apiserver/pkg/admission"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/apiserver/pkg/authentication/user"
)

const ImageRegexpConfigYAML = `{"imageRegexp":[{"regexp":"^(.*?):deployed$","replacement":"$1:deployed-test"}]}`

const ReplacementSuffix = ":deployed-test"

func TestNewFromConfig(t *testing.T) {
	ir, err := NewImageRegexp(strings.NewReader(ImageRegexpConfigYAML))

	if err != nil {
		t.Errorf("Failed to create new ImageRegexp from YAML: %s", err)
		return
	}

	pod := &api.Pod{
		Spec: api.PodSpec{
			ServiceAccountName: "default",
			SecurityContext:    &api.PodSecurityContext{},
			InitContainers: []api.Container{
				{
					Image:           "testregistry.com:8080/user/image:deployed",
				},
				{
					Image:			"testregistry.com:8080/user/image:latest",
				},
			},
			Containers: []api.Container{
				{
					Image:           "testregistry.com:8080/user/image:deployed",
				},
				{
					Image:			"testregistry.com:8080/user/image:latest",
				},
			},
		},
	}

	attr := admission.NewAttributesRecord(pod, nil, api.Kind("Pod").WithVersion("version"), "namespace", "", api.Resource("pods").WithVersion("version"), "", admission.Create, &user.DefaultInfo{})

	if err2 := ir.Admit(attr); err2 != nil {
		t.Errorf("Failed to admit pod: %s", err)
		return
	}

	if !strings.HasSuffix(pod.Spec.InitContainers[0].Image, ReplacementSuffix) {
		t.Errorf("Image '%s' should have suffix '%s'", pod.Spec.Containers[0].Image, ReplacementSuffix)
	}

	if strings.HasSuffix(pod.Spec.InitContainers[1].Image, ReplacementSuffix) {
		t.Errorf("Image '%s' should not have suffix '%s'", pod.Spec.Containers[1].Image, ReplacementSuffix)
	}

	if !strings.HasSuffix(pod.Spec.Containers[0].Image, ReplacementSuffix) {
		t.Errorf("Image '%s' should have suffix '%s'", pod.Spec.Containers[0].Image, ReplacementSuffix)
	}

	if strings.HasSuffix(pod.Spec.Containers[1].Image, ReplacementSuffix) {
		t.Errorf("Image '%s' should not have suffix '%s'", pod.Spec.Containers[1].Image, ReplacementSuffix)
	}
}
