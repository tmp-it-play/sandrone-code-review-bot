package llm

type ProviderPolicyIdentity struct {
	Name               string
	Model              string
	Roles              []TaskRole
	PublicDataAllowed  bool
	PrivateCodeAllowed bool
	ToolCalling        bool
	JSONMode           bool
	PromptLimit        int
	RequestPolicy      ProviderRequestPolicyIdentity
}
