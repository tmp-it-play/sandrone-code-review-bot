package command

type Command struct {
	Kind        Kind
	Invoker     string
	Instruction string
	CommentID   int64
	InThread    bool
}

func (c Command) IsRecognized() bool {
	return c.Kind != KindUnknown
}
