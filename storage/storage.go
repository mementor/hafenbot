package storage

import "github.com/mementor/hafenbot/mtimer"

// Storage interface defines methods of storage drivers
type Storage interface {
	StoreTimer(*mtimer.Timer) error
	GetNearestTimer() (*mtimer.Timer, error)
	GetTimerByID(uint) error
	RemoveTimer(int64, string) error
	GetTimerlistByChatID(int64) ([]*mtimer.Timer, error)
}
