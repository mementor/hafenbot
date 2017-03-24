package storage

import "github.com/mementor/hafenbot/timer"

// Storage interface defines methods of storage drivers
type Storage interface {
	SaveTimer(*timer.Timer) error
	DeleteTimer(int64, string) error
	GetNearestTimer() (*timer.Timer, error)
	ListChatTimers(int64) ([]timer.Timer, error)
	GetTimerByChatAndID(int64, string) (*timer.Timer, error)
	AppendToSSList(chatID int64) error
	DeleteFromSSList(int64)
	GetSSChats() []int64
	// GetMongoStore(string) (*MongoStore, error)
}
