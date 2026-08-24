package llm

type TaskRole string

const (
	TaskRolePlanner  TaskRole = "planner"
	TaskRoleReviewer TaskRole = "reviewer"
	TaskRoleVerifier TaskRole = "verifier"
	TaskRoleReducer  TaskRole = "reducer"
	TaskRoleSummary  TaskRole = "summary"
	TaskRoleReply    TaskRole = "reply"
)

func (r TaskRole) Valid() bool {
	switch r {
	case TaskRolePlanner, TaskRoleReviewer, TaskRoleVerifier, TaskRoleReducer, TaskRoleSummary, TaskRoleReply:
		return true
	default:
		return false
	}
}
