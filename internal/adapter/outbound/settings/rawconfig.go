package settings

type rawConfig struct {
	Language          *string      `yaml:"language"`
	Temperature       *float64     `yaml:"temperature"`
	MaxOutputTokens   *int         `yaml:"maxOutputTokens"`
	MaxPromptChars    *int         `yaml:"maxPromptChars"`
	MaxFiles          *int         `yaml:"maxFiles"`
	MaxFileChars      *int         `yaml:"maxFileChars"`
	IncludeSources    *bool        `yaml:"includeSources"`
	MaxSourceChars    *int         `yaml:"maxSourceChars"`
	MaxExtraReads     *int         `yaml:"maxExtraReads"`
	Exclude           []string     `yaml:"exclude"`
	Include           []string     `yaml:"include"`
	MinSeverity       *string      `yaml:"minSeverity"`
	MaxInlineComments *int         `yaml:"maxInlineComments"`
	ThreadReply       *bool        `yaml:"threadReply"`
	Sandrone          *rawSandrone `yaml:"sandrone"`
}
