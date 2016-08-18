package main

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"flag"

	"github.com/PuerkitoBio/goquery"
	tgbotapi "github.com/Syfaro/telegram-bot-api"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/boltdb/bolt"
	"github.com/satori/go.uuid"
)

// ServerStatus represents current server status
type ServerStatus struct {
	Status       string
	Online       string
	ChangedState chan string
}

func checkHealth(ss *ServerStatus) (status, online string) {
	doc, err := goquery.NewDocument("http://www.havenandhearth.com/portal/")
	if err != nil {
		log.Println(err)
		return
	}
	doc.Find(".vertdiv").Eq(1).Each(func(i int, s *goquery.Selection) {
		status = s.Find("h2").Text()
		online = s.Find("p").Eq(0).Text()
		log.Printf("Status is: %s", status)
		log.Printf("Online is: %s", online)
		if status != "" {
			if online == "" {
				ss.Online = "unknown"
			} else {
				ss.Online = online
			}
			if ss.Status != status {
				oldStatus := ss.Status
				ss.Status = status
				if oldStatus != "" {
					log.Println("Status changed")
					ss.ChangedState <- oldStatus
				}
			}
		}
	})
	return
}

func getSSChats(dynamo *dynamodb.DynamoDB) (chats []int) {
	log.Println("[getSSChats]: Stub!")
	dyParams := &dynamodb.GetItemInput{
		TableName: aws.String("HafenTable"),
		Key: map[string]*dynamodb.AttributeValue{
			"Service": {
				S: aws.String("ServerStatus"),
			},
		},
		ReturnConsumedCapacity: aws.String("TOTAL"),
	}
	resp, err := dynamo.GetItem(dyParams)
	if err != nil {
		log.Println(err.Error())
	}
	fmt.Println(resp)

	for _, chatSTR := range resp.Item["Chats"].NS {
		chatID, _ := strconv.Atoi(*chatSTR)
		log.Printf("Sending to: %d", chatID)
		chats = append(chats, chatID)
	}
	log.Printf("Consumed: %f units", *resp.ConsumedCapacity.CapacityUnits)
	return chats
}

func appendToSSList(dynamo *dynamodb.DynamoDB, chatID int) {
	log.Println("[appendToSSList]: Stub!")
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
	resp, err := dynamo.UpdateItem(dyParams)
	if err != nil {
		log.Println(err.Error())
	}
	log.Println(resp)
}

func deleteFromSSList(dynamo *dynamodb.DynamoDB, chatID int) {
	log.Println("[appendToSSList]: Stub!")
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
	resp, err := dynamo.UpdateItem(dyParams)
	if err != nil {
		log.Println(err.Error())
	}
	log.Println(resp)
}

func main() {
	botToken := flag.String("token", "", "Token to the bot from BotFather")
	flag.Parse()

	db, err := bolt.Open("my.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	dynamo := dynamodb.New(&aws.Config{Region: aws.String("us-east-1")})
	ss := &ServerStatus{}
	ss.ChangedState = make(chan string)
	go checkHealth(ss)
	bot, err := tgbotapi.NewBotAPI(*botToken)
	if err != nil {
		log.Panic(err)
	}
	bot.Debug = false
	log.Printf("Authorized on account %s", bot.Self.UserName)
	ucfg := tgbotapi.NewUpdate(0)
	ucfg.Timeout = 60
	bot.UpdatesChan(ucfg)
	ticker := time.Tick(30 * time.Second)

	for {
		select {
		case update := <-bot.Updates:
			UserName := update.Message.From.UserName
			UserID := update.Message.From.ID
			ChatID := update.Message.Chat.ID
			command := strings.Split(strings.Split(strings.ToLower(update.Message.Text), "@")[0], " ")[0]

			if command == "/status" {
				reply := fmt.Sprintf("Status: %s", ss.Status)
				bot.SendMessage(tgbotapi.NewMessage(ChatID, reply))
			} else if command == "/online" {
				reply := fmt.Sprintf("Online: %s", ss.Online)
				bot.SendMessage(tgbotapi.NewMessage(ChatID, reply))
			} else if command == "/statuson" {
				appendToSSList(dynamo, ChatID)
				reply := "Now you will receive server statuses on server change\n/statusoff to disable"
				bot.SendMessage(tgbotapi.NewMessage(ChatID, reply))
			} else if command == "/statusoff" {
				deleteFromSSList(dynamo, ChatID)
				reply := "Now you will NOT receive server statuses on server change\n/statuson to enable"
				bot.SendMessage(tgbotapi.NewMessage(ChatID, reply))
			} else if command == "/timer" {
				strs := strings.Split(update.Message.Text, " ")
				body := strings.Join(strs[1:len(strs)-1], " ")
				delay := strs[len(strs)-1]
				reply := ""
				duration, err := time.ParseDuration(delay)
				if err != nil {
					reply = fmt.Sprintf("error: '%s'\n", err)
				} else {
					location := time.FixedZone("MSK", 3*60*60)
					fireAt := time.Now().In(location).Add(duration)
					reply = fmt.Sprintf("Timer fire at: '%s'\n%s", fireAt.Format("2006-01-02 15:04:05 MST"), body)
					db.Update(func(tx *bolt.Tx) error {
						parentBucket, erro := tx.CreateBucketIfNotExists([]byte("Timers"))
						if erro != nil {
							fmt.Printf("create parentBucket error: %s", erro)
							return erro
						}
						chatBucket, erro := parentBucket.CreateBucketIfNotExists([]byte(fmt.Sprintf("%d", ChatID)))
						if erro != nil {
							fmt.Printf("create chatBucket error: %s", erro)
							return erro
						}
						u1 := uuid.NewV4()
						fmt.Printf("uuid: %s\n", u1)
						err = chatBucket.Put([]byte(fmt.Sprintf("%d_%s", fireAt.Unix(), u1)), []byte(body))
						if err != nil {
							return err
						}
						return nil
					})
				}
				bot.SendMessage(tgbotapi.NewMessage(ChatID, reply))
			} else if command == "/timerlist" {
				db.View(func(tx *bolt.Tx) error {
					parentBucket := tx.Bucket([]byte("Timers"))
					if parentBucket == nil {
						fmt.Println("parentBucket is nil")
						bot.SendMessage(tgbotapi.NewMessage(ChatID, "parentBucket is nil"))
					}
					chatBucket := parentBucket.Bucket([]byte(fmt.Sprintf("%d", ChatID)))
					if chatBucket == nil {
						fmt.Println("chatBucket is nil")
						bot.SendMessage(tgbotapi.NewMessage(ChatID, "no timers"))
						return nil
					}
					var reply bytes.Buffer
					chatBucket.ForEach(func(k, v []byte) error {
						reply.WriteString(fmt.Sprintf("key=%s, value=%s\n", k, v))
						return nil
					})
					bot.SendMessage(tgbotapi.NewMessage(ChatID, reply.String()))
					return nil
				})
			} else {
				reply := fmt.Sprintf("Unknown command: '%s'", command)
				bot.SendMessage(tgbotapi.NewMessage(ChatID, reply))
			}
			log.Printf("[%s] <%d> (%d) %s", UserName, ChatID, UserID, update.Message.Text)
		case <-ticker:
			go checkHealth(ss)
		case oldStatus := <-ss.ChangedState:
			for _, chatID := range getSSChats(dynamo) {
				log.Printf("Sending to chat %d", chatID)
				msgText := fmt.Sprintf("'%s'\n=>\n'%s'", oldStatus, ss.Status)
				bot.SendMessage(tgbotapi.NewMessage(chatID, msgText))
			}
		}
	}
}
