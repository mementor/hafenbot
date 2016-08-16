package main

import (
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
	"os"
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
	botToken := flag.String("token", "", "Token to the bot")
	flag.Parse()

	dynamo := dynamodb.New(&aws.Config{Region: aws.String("us-east-1")})
	ss := &ServerStatus{}
	ss.ChangedState = make(chan string)
	go checkHealth(ss)
	bot, err := tgbotapi.NewBotAPI(*botToken)
	if err != nil {
		log.Panic(err)
	}
	bot.Debug = true
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
			Text := strings.Split(strings.ToLower(update.Message.Text, "@"))[0]

			if Text == "/status" {
				reply := fmt.Sprintf("Status: %s", ss.Status)
				msg := tgbotapi.NewMessage(ChatID, reply)
				bot.SendMessage(msg)
			} else if Text == "/online" {
				reply := fmt.Sprintf("Online: %s", ss.Online)
				msg := tgbotapi.NewMessage(ChatID, reply)
				bot.SendMessage(msg)
			} else if Text == "/statuson" {
				appendToSSList(dynamo, ChatID)
				reply := "Now you will receive server statuses on server change\n/statusoff to disable"
				msg := tgbotapi.NewMessage(ChatID, reply)
				bot.SendMessage(msg)
			} else if Text == "/statusoff" {
				deleteFromSSList(dynamo, ChatID)
				reply := "Now you will NOT receive server statuses on server change\n/statuson to enable"
				msg := tgbotapi.NewMessage(ChatID, reply)
				bot.SendMessage(msg)
			} else {
				reply := Text
				msg := tgbotapi.NewMessage(ChatID, reply)
				bot.SendMessage(msg)
			}
			log.Printf("[%s] <%d> (%d) %s", UserName, ChatID, UserID, Text)
		case <-ticker:
			go checkHealth(ss)
		case oldStatus := <-ss.ChangedState:
			for _, chatID := range getSSChats(dynamo) {
				log.Printf("Sending to chat %d", chatID)
				msgText := fmt.Sprintf("Status changed from: '%s' to: '%s'", oldStatus, ss.Status)
				msg := tgbotapi.NewMessage(chatID, msgText)
				bot.SendMessage(msg)
			}
		}
	}
}
