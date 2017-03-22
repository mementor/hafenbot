package boltdb

import (
	"fmt"
	"log"

	"github.com/boltdb/bolt"
	"github.com/mementor/hafenbot/mtimer"
	"github.com/mementor/hafenbot/storage"
	uuid "github.com/satori/go.uuid"
)

type boltDB struct {
	db    *bolt.DB
	Debug bool
}

// GetStorage return boltdb storage struct
func GetStorage(path string) (storage.Storage, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, err
	}
	return &boltDB{
		db: db,
	}, nil
}

// GetNearestTimer retrns nearest timer
func (b *boltDB) GetNearestTimer() (*mtimer.Timer, error) {
	log.Print("GetNearestTimer stub")
	return nil, nil
}

// GetTimerById retrns nearest timer
func (b *boltDB) GetTimerByID(id uint) error {
	log.Print("GetTimerByID stub")
	return nil
}

// StoreTimer retrns nearest timer
func (b *boltDB) StoreTimer(mtimer *mtimer.Timer) error {
	log.Print("StoreTimer stub")
	b.db.Update(func(tx *bolt.Tx) error {
		chatID := fmt.Sprintf("%d", mtimer.ChatID)
		bucket := tx.Bucket([]byte(chatID))
		id, _ := bucket.NextSequence()
		uuid := uuid.NewV4()
		mtimer.ID = fmt.Sprintf("%d", id)

		err := bucket.Put([]byte(fmt.Sprintf("%d_%d", mtimer.Time.Unix(), id)), []byte(fmt.Sprintf("%s", uuid)))
		return err
	})
	return nil
}

// RemoveTimer removes the timer by id
func (b *boltDB) RemoveTimer(chatID int64, timerID string) error {
	log.Print("RemoveTimer stub")
	return nil
}

// GetTimerlistByChatID return list of timers for specified chatID
func (b *boltDB) GetTimerlistByChatID(chatID int64) ([]*mtimer.Timer, error) {
	log.Print("GetTimerlistByChatID stub")
	return nil, nil
}
