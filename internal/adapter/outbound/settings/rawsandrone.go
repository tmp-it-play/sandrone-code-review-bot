package settings

type rawSandrone struct {
	Review  rawReviewSetting   `yaml:",inline"`
	Setting rawSandroneSetting `yaml:",inline"`
}
