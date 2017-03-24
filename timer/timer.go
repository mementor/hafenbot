package timer

import "time"

type Timer struct {
	At     time.Time
	Body   string
	ChatID int64
	ID     string
}
