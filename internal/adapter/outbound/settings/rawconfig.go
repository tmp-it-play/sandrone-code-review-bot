package settings

type rawConfig struct {
	Review       rawReviewSetting   `yaml:",inline"`
	RootSandrone rawSandroneSetting `yaml:",inline"`
	Sandrone     *rawSandrone       `yaml:"sandrone"`
}
