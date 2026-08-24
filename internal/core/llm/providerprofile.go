package llm

type ProviderProfile struct {
	Roles              []TaskRole
	PublicDataAllowed  bool
	PrivateCodeAllowed bool
}

func (p ProviderProfile) Allows(role TaskRole, classification DataClassification) bool {
	roleAllowed := false
	for _, allowed := range p.Roles {
		if allowed == role {
			roleAllowed = true
			break
		}
	}
	if !roleAllowed {
		return false
	}
	switch classification {
	case DataClassificationPublic:
		return p.PublicDataAllowed
	case DataClassificationPrivateCode:
		return p.PrivateCodeAllowed
	default:
		return false
	}
}
