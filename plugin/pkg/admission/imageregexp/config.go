package imageregexp

type imageRegexpConfig struct {
	Regexp      string `json:"regexp"`
	Replacement string `json:"replacement"`
	ResolveTag  bool   `json:"resolveTag"`
}

type AdmissionConfig struct {
	ImageRegexpConfigs []imageRegexpConfig `json:"imageRegexp"`
}
