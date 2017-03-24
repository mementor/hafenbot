package mongodb

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"github.com/mementor/hafenbot/storage"
	"github.com/mementor/hafenbot/timer"
	uuid "github.com/satori/go.uuid"

	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// MongoStore implements Store interface and communicate to MongoDB
type MongoStore struct {
	msess *mgo.Session
}

// GetMongoStore returns prepared Store
func GetMongoStore(uri string) (storage.Storage, error) {
	mstore := &MongoStore{}
	if uri == "" {
		return mstore, errors.New("No uri specified")
	}
	uri = strings.TrimSuffix(uri, "?ssl=true")

	tlsConfig := &tls.Config{}
	tlsConfig.InsecureSkipVerify = true

	dialInfo, err := mgo.ParseURL(uri)

	if err != nil {
		return mstore, err
	}

	dialInfo.DialServer = func(addr *mgo.ServerAddr) (net.Conn, error) {
		conn, errDial := tls.Dial("tcp", addr.String(), tlsConfig)
		return conn, errDial
	}

	msess, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		fmt.Println("Failed to connect: ", err)
		os.Exit(1)
	}

	mstore.msess = msess
	mstore.msess.SetMode(mgo.Monotonic, true)
	return mstore, nil
}

// SaveTimer saves the timer into MongoDB
func (mstore *MongoStore) SaveTimer(timer *timer.Timer) error {
	timer.ID = fmt.Sprintf("%s", uuid.NewV4())
	TimersCollection := mstore.msess.DB("TimerBot").C("timers")
	err := TimersCollection.Insert(timer)
	if err != nil {
		return err
	}
	return nil
}

// DeleteTimer deletes the timer from MongoDB by ChatID and ID
func (mstore *MongoStore) DeleteTimer(chatID int64, ID string) error {
	// log.Printf("[deleteTimer]: ChatID: %d, ID: %s\n", ChatID, ID)
	timer, err := mstore.GetTimerByChatAndID(chatID, ID)
	if err != nil {
		return err
	}
	if timer == nil {
		return errors.New("No such timer")
	}

	TimersCollection := mstore.msess.DB("TimerBot").C("timers")
	err = TimersCollection.Remove(bson.M{"id": timer.ID})
	if err != nil {
		return err
	}
	return nil
}

// GetTimerByChatAndID returns timer by ChatID and ID from MongoDB
func (mstore *MongoStore) GetTimerByChatAndID(chatID int64, ID string) (timer *timer.Timer, err error) {
	TimersCollection := mstore.msess.DB("TimerBot").C("timers")
	filters := bson.M{
		"chatid": chatID,
		"id":     ID,
	}
	err = TimersCollection.Find(filters).One(&timer)

	if err != nil {
		log.Println(err.Error())
	}
	return
}

// GetNearestTimer returns first timer in MongoDB by fire time
func (mstore *MongoStore) GetNearestTimer() (timer *timer.Timer, err error) {
	TimersCollection := mstore.msess.DB("TimerBot").C("timers")
	err = TimersCollection.Find(bson.M{}).Sort("at").One(&timer)
	if err != nil && err.Error() != "not found" {
		return
	}
	return timer, nil
}

// ListChatTimers returns array of timers by ChatID ordered by time
func (mstore *MongoStore) ListChatTimers(chatID int64) (timers []timer.Timer, err error) {
	TimersCollection := mstore.msess.DB("TimerBot").C("timers")
	err = TimersCollection.Find(bson.M{"chatid": chatID}).Sort("at").All(&timers)
	log.Printf("timer: %v", timers)

	if err != nil {
		log.Println(err.Error())
	}
	return
}

// AppendToSSList adds chatID to list of subscribtions of server status changes
func (mstore *MongoStore) AppendToSSList(chatID int64) error {
	SubsCollection := mstore.msess.DB("TimerBot").C("subs")
	changeInfo, err := SubsCollection.UpsertId(chatID, bson.M{"chat": chatID})
	log.Printf("%+v", changeInfo)
	if changeInfo.Updated > 0 {
		return errors.New("Already subscribed")
	}
	if err != nil {
		log.Println(err.Error())
	}
	return err
}

// DeleteFromSSList removes chatID from list of subscriptions of server status changes
func (mstore *MongoStore) DeleteFromSSList(chatID int64) {
	SubsCollection := mstore.msess.DB("TimerBot").C("subs")
	err := SubsCollection.RemoveId(chatID)
	if err != nil {
		log.Println(err.Error())
	}
}

// GetSSChats return array of chats subscribed to server status changes
func (mstore *MongoStore) GetSSChats() (chats []int64) {
	type Ch struct {
		Chat int64
	}
	var ch []Ch
	SubsCollection := mstore.msess.DB("TimerBot").C("subs")
	err := SubsCollection.Find(nil).All(&ch)
	if err != nil {
		log.Printf("err: %s", err)
	}
	log.Printf("ch: %+v", &ch)
	for _, val := range ch {
		log.Printf("val: %+v", val)
		chats = append(chats, val.Chat)
	}
	log.Printf("chats: %+v", chats)
	return
}
