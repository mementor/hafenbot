package dynamodb

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/mementor/hafenbot/storage"
	"github.com/mementor/hafenbot/timer"
	uuid "github.com/satori/go.uuid"
)

// DynamoStore implements Store interface and communicate to DynamoDB
type DynamoStore struct {
	db *dynamodb.DynamoDB
}

// GetMongoStore returns prepared Store
func GetDynamoStore() (storage.Storage, error) {
	dyn := &DynamoStore{}

	sess, err := session.NewSession(&aws.Config{Region: aws.String("us-east-1")})
	if err != nil {
		return dyn, err
	}
	dyn.db = dynamodb.New(sess)
	return dyn, nil
}

func (dyn *DynamoStore) GetSSChats() (chats []int64) {
	// log.Println("[getSSChats]: Stub!")
	dyParams := &dynamodb.GetItemInput{
		TableName: aws.String("HafenTable"),
		Key: map[string]*dynamodb.AttributeValue{
			"Service": {
				S: aws.String("ServerStatus"),
			},
		},
		ReturnConsumedCapacity: aws.String("TOTAL"),
	}
	resp, err := dyn.db.GetItem(dyParams)
	if err != nil {
		log.Println(err.Error())
	}
	fmt.Println(resp)

	for _, chatSTR := range resp.Item["Chats"].NS {
		chatID, _ := strconv.ParseInt(*chatSTR, 10, 64)
		log.Printf("Sending to: %d", chatID)
		chats = append(chats, chatID)
	}
	log.Printf("Consumed: %f units", *resp.ConsumedCapacity.CapacityUnits)
	return chats
}

func (dyn *DynamoStore) AppendToSSList(chatID int64) (err error) {
	// log.Println("[appendToSSList]: Stub!")
	chatIDStr := fmt.Sprintf("%d", chatID)
	dyParams := &dynamodb.UpdateItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"Service": {
				S: aws.String("ServerStatus"),
			},
		},
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":val1": {NS: aws.StringSlice([]string{chatIDStr})},
		},
		UpdateExpression: aws.String("add Chats :val1"),
		TableName:        aws.String("HafenTable"),
	}
	_, err = dyn.db.UpdateItem(dyParams)
	if err != nil {
		return
	}
	return
}

func (dyn *DynamoStore) DeleteFromSSList(chatID int64) {
	// log.Println("[appendToSSList]: Stub!")
	chatIDStr := fmt.Sprintf("%d", chatID)
	dyParams := &dynamodb.UpdateItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"Service": {
				S: aws.String("ServerStatus"),
			},
		},
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":val1": {NS: aws.StringSlice([]string{chatIDStr})},
		},
		UpdateExpression: aws.String("delete Chats :val1"),
		TableName:        aws.String("HafenTable"),
	}
	_, err := dyn.db.UpdateItem(dyParams)
	if err != nil {
		log.Println(err.Error())
	}
	// log.Println(resp)
}

func (dyn *DynamoStore) SaveTimer(timer *timer.Timer) error {
	// log.Println("[saveTimer]: Stub!")
	dyParams := &dynamodb.PutItemInput{
		TableName: aws.String("HafenAlarms"),
		Item: map[string]*dynamodb.AttributeValue{
			"dt": {
				N: aws.String(fmt.Sprintf("%d", timer.At.Unix())),
			},
			"id": {
				S: aws.String(fmt.Sprintf("%s", uuid.NewV4())),
			},
			"chatid": {
				N: aws.String(fmt.Sprintf("%d", timer.ChatID)),
			},
			"body": {
				S: aws.String(timer.Body),
			},
			"enabled": {
				N: aws.String("1"),
			},
		},
	}
	_, err := dyn.db.PutItem(dyParams)
	if err != nil {
		return err
	}
	// log.Println(resp)
	return nil
}

func (dyn *DynamoStore) ListChatTimers(ChatID int64) (timers []timer.Timer, err error) {
	dyParams := &dynamodb.QueryInput{
		TableName:              aws.String("HafenAlarms"),
		IndexName:              aws.String("chatid-dt-index"),
		KeyConditionExpression: aws.String("chatid = :chtid"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":chtid": {
				N: aws.String(fmt.Sprintf("%d", ChatID)),
			},
		},
	}
	resp, err := dyn.db.Query(dyParams)
	if err != nil {
		log.Println(err.Error())
	}
	// log.Println(resp)
	for _, items := range resp.Items {
		chatid, _ := strconv.ParseInt(*items["chatid"].N, 10, 64)
		timestamp, _ := strconv.ParseInt(*items["dt"].N, 10, 64)
		body := *items["body"].S
		id := *items["id"].S
		timers = append(timers, timer.Timer{ChatID: chatid, At: time.Unix(timestamp, 0), Body: body, ID: id})
	}
	return
}

func (dyn *DynamoStore) GetTimerByChatAndID(ChatID int64, ID string) (rtimer *timer.Timer, err error) {
	// log.Printf("[listTimers]: ChatID: %d, ID: %s\n", ChatID, ID)
	dyParams := &dynamodb.QueryInput{
		TableName:              aws.String("HafenAlarms"),
		IndexName:              aws.String("chatid-dt-index"),
		KeyConditionExpression: aws.String("chatid = :chtid"),
		FilterExpression:       aws.String("id = :id"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":chtid": {
				N: aws.String(fmt.Sprintf("%d", ChatID)),
			},
			":id": {
				S: aws.String(ID),
			},
		},
		Limit: aws.Int64(1),
	}
	resp, err := dyn.db.Query(dyParams)
	if err != nil {
		log.Println(err.Error())
	}
	// log.Println(resp)
	for _, items := range resp.Items {
		chatid, _ := strconv.ParseInt(*items["chatid"].N, 10, 64)
		timestamp, _ := strconv.ParseInt(*items["dt"].N, 10, 64)
		body := *items["body"].S
		id := *items["id"].S

		rtimer = &timer.Timer{
			ChatID: chatid,
			At:     time.Unix(timestamp, 0),
			Body:   body,
			ID:     id}
		return
	}
	return
}

func (dyn *DynamoStore) GetNearestTimer() (rtimer *timer.Timer, err error) {
	// log.Println("[getNearestTimer]: Stub!")
	// epoch := time.Now().Unix()
	dyParams := &dynamodb.QueryInput{
		TableName:              aws.String("HafenAlarms"),
		IndexName:              aws.String("enabled-dt-index"),
		KeyConditionExpression: aws.String("enabled = :nbl"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			// ":dt": {
			// 	N: aws.String(fmt.Sprintf("%d", epoch)),
			// },
			":nbl": {
				N: aws.String("1"),
			},
		},
		Limit: aws.Int64(1),
	}
	resp, err := dyn.db.Query(dyParams)
	if err != nil {
		return
	}
	// log.Println(resp)
	for _, items := range resp.Items {
		chatid, _ := strconv.ParseInt(*items["chatid"].N, 10, 64)
		timestamp, _ := strconv.ParseInt(*items["dt"].N, 10, 64)
		body := *items["body"].S
		id := *items["id"].S
		rtimer = &timer.Timer{
			ChatID: chatid,
			At:     time.Unix(timestamp, 0),
			Body:   body,
			ID:     id}
		return
	}
	return
}

func (dyn *DynamoStore) DeleteTimer(ChatID int64, ID string) error {
	// log.Printf("[deleteTimer]: ChatID: %d, ID: %s\n", ChatID, ID)
	rtimer, err := dyn.GetTimerByChatAndID(ChatID, ID)
	if err != nil {
		return err
	}
	if rtimer == nil {
		return errors.New("No such timer")
	}
	dyParams := &dynamodb.DeleteItemInput{
		TableName: aws.String("HafenAlarms"),
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(rtimer.ID),
			},
		},
	}
	_, err = dyn.db.DeleteItem(dyParams)
	if err != nil {
		return err
	}
	return nil
}
