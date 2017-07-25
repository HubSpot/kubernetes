package imageregexp

type imageRegexpConfig struct {
	Regexp string `json:"regexp"`
	Replacement   string `json:"replacement"`
}

type AdmissionConfig struct {
	ImageRegexpConfigs []imageRegexpConfig `json:"imageRegexp"`
}
