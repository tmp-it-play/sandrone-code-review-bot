package settings

type rawConfig struct {
	Review   rawReviewSetting `yaml:",inline"`
	Sandrone *rawSandrone     `yaml:"sandrone"`
}
