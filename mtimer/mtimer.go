package mtimer

import "time"

// Timer struct represents timer entry
type Timer struct {
	Time   time.Time
	Body   string
	ChatID int64
	ID     string
	Done   bool
}
