package thread

import "time"

type Message struct {
	Author    string
	Body      string
	FromBot   bool
	CreatedAt time.Time
}
