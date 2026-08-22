package thread

type Thread struct {
	RootCommentID int64
	ReplyToID     int64
	Path          string
	Line          int
	DiffHunk      string
	Messages      []Message
}

func (t Thread) Origin() (Message, bool) {
	if len(t.Messages) == 0 {
		return Message{}, false
	}
	return t.Messages[0], true
}

func (t Thread) LastHumanRequest() (Message, bool) {
	for index := len(t.Messages) - 1; index >= 0; index-- {
		if !t.Messages[index].FromBot {
			return t.Messages[index], true
		}
	}
	return Message{}, false
}
