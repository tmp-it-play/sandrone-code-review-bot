package worker

func outcomeLabel(err error) string {
	if err != nil {
		return "failed"
	}
	return "succeeded"
}
