package llm

const defaultOperationCallLimit = 24
const lightweightOperationCallLimit = 8

func OperationCallLimit(configured int, role TaskRole) int {
	if configured < 1 {
		configured = defaultOperationCallLimit
	}
	if (role == TaskRoleSummary || role == TaskRoleReply) && configured > lightweightOperationCallLimit {
		return lightweightOperationCallLimit
	}
	return configured
}
