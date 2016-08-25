package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"flag"

	"github.com/PuerkitoBio/goquery"
	tgbotapi "github.com/Syfaro/telegram-bot-api"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/satori/go.uuid"
)

// ServerStatus represents current server status
type ServerStatus struct {
	Status       string
	Online       string
	ChangedState chan string
}

// Timer struct represents timer entry
type Timer struct {
	Time   time.Time
	Body   string
	ChatID int
	ID     string
}

var location *time.Location

func checkHealth(ss *ServerStatus) {
	doc, err := goquery.NewDocument("http://www.havenandhearth.com/portal/")
	if err != nil {
		log.Println(err)
		return
	}
	doc.Find(".vertdiv").Eq(1).Each(func(i int, s *goquery.Selection) {
		status := s.Find("h2").Text()
		online := s.Find("p").Eq(0).Text()
		// log.Printf("Status is: %s", status)
		// log.Printf("Online is: %s", online)
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

func forTheWatch(dynamo *dynamodb.DynamoDB, bot *tgbotapi.BotAPI, reload chan bool) {
	defer log.Println("John Snow died...")
	log.Println("Its my watch")
	var reply string
	var wg sync.WaitGroup
	reload <- true
	ticker := time.Tick(10 * time.Second)
	nearestID := ""
	for {
		select {
		case <-reload:
			wg.Wait()
			wg.Add(1)
			timer, err := getNearestTimer(dynamo)
			if err != nil {
				log.Println(err)
			} else if timer != nil {
				if timer.Time.Before(time.Now()) {
					err = deleteTimer(dynamo, timer.ChatID, timer.ID)
					if err != nil {
						log.Println(err)
					} else {
						reply = fmt.Sprintf("⏰ %s\n%s", timer.Time.In(location).Format("2006-01-02 15:04:05 MST"), timer.Body)
						bot.SendMessage(tgbotapi.NewMessage(timer.ChatID, reply))
						reload <- true
					}
				} else {
					if timer.ID != nearestID {
						log.Printf("sleep for %s", timer.Time.Sub(time.Now()))
						nearestID = timer.ID
						time.AfterFunc(timer.Time.Sub(time.Now()), func() {
							reload <- true
						})
					}
				}
			}
			wg.Done()
		case <-ticker:
			reload <- true
		}
	}
}

func getSSChats(dynamo *dynamodb.DynamoDB) (chats []int) {
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
	_, err := dynamo.UpdateItem(dyParams)
	if err != nil {
		log.Println(err.Error())
	}
	// log.Println(resp)
}

func deleteFromSSList(dynamo *dynamodb.DynamoDB, chatID int) {
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
	_, err := dynamo.UpdateItem(dyParams)
	if err != nil {
		log.Println(err.Error())
	}
	// log.Println(resp)
}

func saveTimer(dynamo *dynamodb.DynamoDB, timer *Timer) error {
	// log.Println("[saveTimer]: Stub!")
	dyParams := &dynamodb.PutItemInput{
		TableName: aws.String("HafenAlarms"),
		Item: map[string]*dynamodb.AttributeValue{
			"dt": {
				N: aws.String(fmt.Sprintf("%d", timer.Time.Unix())),
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
	_, err := dynamo.PutItem(dyParams)
	if err != nil {
		return err
	}
	// log.Println(resp)
	return nil
}

func listChatTimers(dynamo *dynamodb.DynamoDB, ChatID int) (timers []*Timer, err error) {
	return listTimersByChatAndID(dynamo, ChatID, "")
}

func listTimersByChatAndID(dynamo *dynamodb.DynamoDB, ChatID int, ID string) (timers []*Timer, err error) {
	// log.Printf("[listTimers]: ChatID: %d, ID: %s\n", ChatID, ID)
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
	if ID != "" {
		dyParams.FilterExpression = aws.String("id = :id")
		dyParams.ExpressionAttributeValues[":id"] = &dynamodb.AttributeValue{S: aws.String(ID)}
	}
	resp, err := dynamo.Query(dyParams)
	if err != nil {
		log.Println(err.Error())
	}
	// log.Println(resp)
	for _, items := range resp.Items {
		chatid, _ := strconv.Atoi(*items["chatid"].N)
		timestamp, _ := strconv.ParseInt(*items["dt"].N, 10, 64)
		body := *items["body"].S
		id := *items["id"].S
		timers = append(timers, &Timer{ChatID: chatid, Time: time.Unix(timestamp, 0), Body: body, ID: id})
	}
	return
}

func getNearestTimer(dynamo *dynamodb.DynamoDB) (timer *Timer, err error) {
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
	resp, err := dynamo.Query(dyParams)
	if err != nil {
		return
	}
	// log.Println(resp)
	for _, items := range resp.Items {
		chatid, _ := strconv.Atoi(*items["chatid"].N)
		timestamp, _ := strconv.ParseInt(*items["dt"].N, 10, 64)
		body := *items["body"].S
		id := *items["id"].S
		timer = &Timer{
			ChatID: chatid,
			Time:   time.Unix(timestamp, 0),
			Body:   body,
			ID:     id}
		return
	}
	return
}

func deleteTimer(dynamo *dynamodb.DynamoDB, ChatID int, ID string) error {
	// log.Printf("[deleteTimer]: ChatID: %d, ID: %s\n", ChatID, ID)
	timers, err := listTimersByChatAndID(dynamo, ChatID, ID)
	if err != nil {
		return err
	}
	if len(timers) < 1 {
		return errors.New("No such timer")
	}
	dyParams := &dynamodb.DeleteItemInput{
		TableName: aws.String("HafenAlarms"),
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(timers[0].ID),
			},
		},
	}
	_, err = dynamo.DeleteItem(dyParams)
	if err != nil {
		return err
	}
	return nil
}

func parseDuration(str string) (dur time.Duration, err error) {
	var overall int
	reWeeks := regexp.MustCompile("^(\\d(.\\d+)?)w$")
	reDays := regexp.MustCompile("^(\\d(.\\d+)?)d$")
	reHours := regexp.MustCompile("^(\\d(.\\d+)?)h$")
	reMinutes := regexp.MustCompile("^(\\d(.\\d+)?)m$")
	reSeconds := regexp.MustCompile("^(\\d(.\\d+)?)s$")

	str = regexp.MustCompile("([wdhms])(\\d)").ReplaceAllString(str, "$1,$2")
	fmt.Printf("replaced: %s\n", str)
	tockens := strings.Split(str, ",")
	fmt.Printf("tockens: %s\n", tockens)
	for _, to := range tockens {
		if reWeeks.MatchString(to) {
			weeks, err1 := strconv.ParseFloat(reWeeks.ReplaceAllString(to, "$1"), 10)
			if err1 != nil {
				fmt.Printf("err: %s", err)
				return dur, err1
			}
			overall += int(weeks * 7 * 24 * 60 * 60)
		} else if reDays.MatchString(to) {
			days, err1 := strconv.ParseFloat(reDays.ReplaceAllString(to, "$1"), 10)
			if err1 != nil {
				fmt.Printf("err: %s", err)
				return dur, err1
			}
			overall += int(days * 24 * 60 * 60)
		} else if reHours.MatchString(to) {
			hours, err1 := strconv.ParseFloat(reHours.ReplaceAllString(to, "$1"), 10)
			if err1 != nil {
				fmt.Printf("err: %s", err)
				return dur, err1
			}
			overall += int(hours * 60 * 60)
		} else if reMinutes.MatchString(to) {
			minutes, err1 := strconv.ParseFloat(reMinutes.ReplaceAllString(to, "$1"), 10)
			if err1 != nil {
				fmt.Printf("err: %s", err)
				return dur, err1
			}
			overall += int(minutes * 60)
		} else if reSeconds.MatchString(to) {
			seconds, err1 := strconv.ParseFloat(reSeconds.ReplaceAllString(to, "$1"), 10)
			if err1 != nil {
				fmt.Printf("err: %s", err)
				return dur, err1
			}
			overall += int(seconds)
		}
	}
	if overall == 0 {
		return dur, errors.New("Cant parse duration")
	}
	dur, err = time.ParseDuration(fmt.Sprintf("%ds", overall))
	return
}

func parseDateTime(str string) (t time.Time, err error) {
	// 2006-01-02 15:04:05 MST
	fmt.Printf("parseDateTime\n")
	fullFormats := []string{"20060102 15:04", "20060102 15:04:05", "02.01.2006 15:04", "02.01.2006 15:04:05"}
	partFormats := []string{"15:04", "15:04:05"}
	for _, format := range fullFormats {
		t, err := time.ParseInLocation(format, str, location)
		if err == nil {
			fmt.Printf("parsed time: %s\n", t)
			return t, nil
		}
		fmt.Printf("err1: %s\n", err)
	}
	for _, format := range partFormats {
		tim, err := time.ParseInLocation(format, str, location)
		if err == nil {
			now := time.Now()
			parsedToday := time.Date(now.Year(), now.Month(), now.Day(), tim.Hour(), tim.Minute(), tim.Second(), 0, location)
			if time.Now().After(parsedToday) {
				parsedToday = parsedToday.Add(24 * time.Hour)
			}
			return parsedToday, nil
		}
	}
	return time.Time{}, errors.New("Cant parse datetime")
}

func main() {
	botToken := flag.String("token", "", "Token to the bot")
	debug := flag.Bool("debug", false, "Debug to stdout")
	flag.Parse()

	location = time.FixedZone("MSK", 3*60*60)

	dynamo := dynamodb.New(&aws.Config{Region: aws.String("us-east-1")})
	ss := &ServerStatus{}
	ss.ChangedState = make(chan string)
	bot, err := tgbotapi.NewBotAPI(*botToken)
	if err != nil {
		log.Panic(err)
	}
	bot.Debug = *debug
	log.Printf("Authorized on account %s", bot.Self.UserName)
	ucfg := tgbotapi.NewUpdate(0)
	ucfg.Timeout = 60
	bot.UpdatesChan(ucfg)
	ticker := time.Tick(30 * time.Second)
	reload := make(chan bool, 100)
	go checkHealth(ss)
	go forTheWatch(dynamo, bot, reload)

	for {
		select {
		case update := <-bot.Updates:
			UserName := update.Message.From.UserName
			UserID := update.Message.From.ID
			ChatID := update.Message.Chat.ID
			strs := strings.Split(update.Message.Text, " ")
			command := strings.Split(strings.ToLower(strs[0]), "@")[0]
			body := strings.Join(strs[1:], " ")

			if command == "" {
				continue
			} else if command == "/status" {
				reply := fmt.Sprintf("Status: %s", ss.Status)
				msg := tgbotapi.NewMessage(ChatID, reply)
				bot.SendMessage(msg)
			} else if command == "/online" {
				reply := fmt.Sprintf("Online: %s", ss.Online)
				msg := tgbotapi.NewMessage(ChatID, reply)
				bot.SendMessage(msg)
			} else if command == "/statuson" {
				appendToSSList(dynamo, ChatID)
				reply := "Now you will receive server statuses on server change\n/statusoff to disable"
				msg := tgbotapi.NewMessage(ChatID, reply)
				bot.SendMessage(msg)
			} else if command == "/statusoff" {
				deleteFromSSList(dynamo, ChatID)
				reply := "Now you will NOT receive server statuses on server change\n/statuson to enable"
				msg := tgbotapi.NewMessage(ChatID, reply)
				bot.SendMessage(msg)
			} else if command == "/timer" { // dirty shit
				if len(strs) < 3 {
					bot.SendMessage(tgbotapi.NewMessage(ChatID, "send me timer in following format:\n /timer text 15m"))
					continue
				}
				description := strings.Join(strs[1:len(strs)-1], " ")
				delayTwoWords := strings.Join(strs[len(strs)-2:], " ")
				delay := strs[len(strs)-1]
				reply := ""
				ok := false
				duration, err := parseDuration(delay)
				var fireAt time.Time
				if err != nil {
					fireAt, err = parseDateTime(delayTwoWords)
					description = strings.Join(strs[1:len(strs)-2], " ")
					if err != nil {
						fmt.Printf("err1: %s\n", err)
						fireAt, err = parseDateTime(delay)
						description = strings.Join(strs[1:len(strs)-1], " ")
						if err != nil {
							reply = fmt.Sprintf("error: '%s'\n", err)
						}
					}
					if fireAt.Before(time.Now()) {
						reply = fmt.Sprintf("error: time is in past")
					} else {
						ok = true
					}
				} else {
					fireAt = time.Now().Add(duration)
					ok = true
				}

				if ok {
					if description == "" {
						reply = "error: timer have no text"
					} else {
						timer := &Timer{
							Time:   fireAt,
							Body:   description,
							ChatID: ChatID,
						}
						err = saveTimer(dynamo, timer)
						if err != nil {
							log.Println(err.Error())
							reply = fmt.Sprintf("error:\n%s", err)
						} else {
							reply = fmt.Sprintf("⏲ fire at %s", fireAt.In(location).Format("2006-01-02 15:04:05 MST"))
							reload <- true
						}
					}
				}
				bot.SendMessage(tgbotapi.NewMessage(ChatID, reply))
			} else if command == "/timerlist" {
				timers, _ := listChatTimers(dynamo, ChatID)
				var reply bytes.Buffer
				if len(timers) == 0 {
					reply.WriteString("No timers here yet")
				} else {
					for i, t := range timers {
						reply.WriteString(fmt.Sprintf("⏲ %s\n%s\n%s\n\n", t.Time.In(location).Format("2006-01-02 15:04:05 MST"), t.Body, t.ID))
						fmt.Printf("timer[%d]: '%v'\n", i, t)
					}
				}
				bot.SendMessage(tgbotapi.NewMessage(ChatID, reply.String()))
			} else if command == "/timerdel" {
				err := deleteTimer(dynamo, ChatID, body)
				if err != nil {
					bot.SendMessage(tgbotapi.NewMessage(ChatID, err.Error()))
				}
				bot.SendMessage(tgbotapi.NewMessage(ChatID, "Done!"))
			} else {
				reply := fmt.Sprintf("Unknown command: '%s'", command)
				msg := tgbotapi.NewMessage(ChatID, reply)
				bot.SendMessage(msg)
			}
			log.Printf("[%s] <%d> (%d) %s", UserName, ChatID, UserID, update.Message.Text)
		case <-ticker:
			go checkHealth(ss)
		case oldStatus := <-ss.ChangedState:
			for _, chatID := range getSSChats(dynamo) {
				log.Printf("Sending to chat %d", chatID)
				msgText := fmt.Sprintf("'%s'\n=>\n'%s'", oldStatus, ss.Status)
				msg := tgbotapi.NewMessage(chatID, msgText)
				bot.SendMessage(msg)
			}
		}
	}
}
