package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"flag"

	"github.com/PuerkitoBio/goquery"
	"github.com/mementor/hafenbot/storage"
	"github.com/mementor/hafenbot/storage/dynamodb"
	"github.com/mementor/hafenbot/storage/mongodb"
	"github.com/mementor/hafenbot/timer"
	tgbotapi "gopkg.in/telegram-bot-api.v4"
)

const (
	done   = "done"
	undone = "undone"
)

// ServerStatus represents current server status
type ServerStatus struct {
	Status       string
	Online       string
	ChangedState chan string
}

type button struct {
	isDone bool
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

func getInlineKeyboard(btn button) (keyboard *tgbotapi.InlineKeyboardMarkup) {
	text := "✗"
	data := undone
	if btn.isDone {
		text = "✓"
		data = done
	}
	keyboard = &tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
			[]tgbotapi.InlineKeyboardButton{
				tgbotapi.InlineKeyboardButton{
					Text:         text,
					CallbackData: &data,
				},
			},
		},
	}
	return
}

func forTheWatch(store storage.Storage, bot *tgbotapi.BotAPI, reload chan bool) {
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
			timer, err := store.GetNearestTimer()
			if err != nil {
				log.Println(err)
			} else if timer != nil {
				if timer.At.Before(time.Now()) {
					err = store.DeleteTimer(timer.ChatID, timer.ID)
					if err != nil {
						log.Println(err)
					} else {
						reply = fmt.Sprintf("⏰ %s\n%s", timer.At.In(location).Format("2006-01-02 15:04:05 MST"), timer.Body)
						msg := tgbotapi.NewMessage(int64(timer.ChatID), reply)
						msg.BaseChat.ReplyMarkup = getInlineKeyboard(button{isDone: false})
						bot.Send(msg)
						reload <- true
					}
				} else {
					if timer.ID != nearestID {
						log.Printf("sleep for %s", timer.At.Sub(time.Now()))
						nearestID = timer.ID
						time.AfterFunc(timer.At.Sub(time.Now()), func() {
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

func parseDuration(str string) (dur time.Duration, err error) {
	var overall int
	reWeeks := regexp.MustCompile("^(\\d+(.\\d+)?)w$")
	reDays := regexp.MustCompile("^(\\d+(.\\d+)?)d$")
	reHours := regexp.MustCompile("^(\\d+(.\\d+)?)h$")
	reMinutes := regexp.MustCompile("^(\\d+(.\\d+)?)m$")
	reSeconds := regexp.MustCompile("^(\\d+(.\\d+)?)s$")

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
				log.Printf("err: %s", err)
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
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	var mongosrv string
	var botToken string
	var dbdriver string
	var debug bool
	flag.StringVar(&botToken, "token", "", "Token to the bot")
	flag.StringVar(&dbdriver, "dbdriver", "", "Database driver to use (mongo or dynamo)")
	flag.StringVar(&mongosrv, "mongosrv", "", "Address of mongo servers")
	flag.BoolVar(&debug, "debug", false, "Debug to stdout")

	flag.Parse()

	var dbstore storage.Storage
	var err error

	if dbdriver == "mongo" {
		dbstore, err = mongodb.GetMongoStore(mongosrv)
		if err != nil {
			log.Print(err)
			os.Exit(1)
		}
	} else if dbdriver == "dynamo" {
		dbstore, err = dynamodb.GetDynamoStore()
		if err != nil {
			log.Print(err)
			os.Exit(1)
		}
	} else {
		log.Fatal("No such --dbdriver")
	}

	location = time.FixedZone("MSK", 3*60*60)

	ss := &ServerStatus{}
	ss.ChangedState = make(chan string)
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panic(err)
	}
	bot.Debug = debug
	log.Printf("Authorized on account %s", bot.Self.UserName)
	ucfg := tgbotapi.NewUpdate(0)
	ucfg.Timeout = 60
	updates, err := bot.GetUpdatesChan(ucfg)
	if err != nil {
		log.Panic(err)
		return
	}
	ticker := time.Tick(30 * time.Second)
	reload := make(chan bool, 100)
	go checkHealth(ss)
	go forTheWatch(dbstore, bot, reload)

	for {
		select {
		case update := <-updates:
			log.Printf("%+v", update)
			if update.CallbackQuery != nil {
				buttonData := done
				buttonText := "✓"
				newMsgText := strings.Replace(update.CallbackQuery.Message.Text, "⏰", "✓", 1)
				if update.CallbackQuery.Data == done {
					buttonData = undone
					buttonText = "✗"
					newMsgText = strings.Replace(update.CallbackQuery.Message.Text, "✓", "⏰", 1)
				}
				markup := tgbotapi.InlineKeyboardMarkup{
					InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
						[]tgbotapi.InlineKeyboardButton{
							tgbotapi.InlineKeyboardButton{
								Text:         buttonText,
								CallbackData: &buttonData,
							},
						},
					},
				}
				editConfig := tgbotapi.EditMessageTextConfig{
					BaseEdit: tgbotapi.BaseEdit{
						ChatID:      update.CallbackQuery.Message.Chat.ID,
						MessageID:   update.CallbackQuery.Message.MessageID,
						ReplyMarkup: &markup,
					},
					Text: newMsgText,
				}
				log.Printf("%+v", editConfig)
				bot.Send(editConfig)
			}
			if update.Message == nil {
				continue
			}
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
				bot.Send(msg)
			} else if command == "/online" {
				reply := fmt.Sprintf("Online: %s", ss.Online)
				msg := tgbotapi.NewMessage(ChatID, reply)
				bot.Send(msg)
			} else if command == "/statuson" {
				err := dbstore.AppendToSSList(ChatID)
				var reply string
				if err == nil {
					reply = "Now you will receive server statuses on server change\n/statusoff to disable"
				} else {
					reply = fmt.Sprintf("Error: %s", err)
				}
				msg := tgbotapi.NewMessage(ChatID, reply)
				bot.Send(msg)
			} else if command == "/statusoff" {
				dbstore.DeleteFromSSList(ChatID)
				reply := "Now you will NOT receive server statuses on server change\n/statuson to enable"
				msg := tgbotapi.NewMessage(ChatID, reply)
				bot.Send(msg)
			} else if command == "/timer" { // dirty shit
				if len(strs) < 3 {
					bot.Send(tgbotapi.NewMessage(ChatID, "send me timer in following format:\n /timer text 15m"))
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
						timer := &timer.Timer{
							At:     fireAt,
							Body:   description,
							ChatID: ChatID,
						}
						err = dbstore.SaveTimer(timer)
						if err != nil {
							log.Println(err.Error())
							reply = fmt.Sprintf("error:\n%s", err)
						} else {
							reply = fmt.Sprintf("⏲ fire at %s", fireAt.In(location).Format("2006-01-02 15:04:05 MST"))
							reload <- true
						}
					}
				}
				bot.Send(tgbotapi.NewMessage(ChatID, reply))
			} else if command == "/timerlist" {
				timers, _ := dbstore.ListChatTimers(ChatID)
				var reply bytes.Buffer
				if len(timers) == 0 {
					reply.WriteString("No timers here yet")
				} else {
					for i, t := range timers {
						reply.WriteString(fmt.Sprintf("⏲ %s\n%s\n%s\n\n", t.At.In(location).Format("2006-01-02 15:04:05 MST"), t.Body, t.ID))
						log.Printf("timer[%d]: '%v'\n", i, t)
					}
				}
				bot.Send(tgbotapi.NewMessage(ChatID, reply.String()))
			} else if command == "/timerdel" {
				err := dbstore.DeleteTimer(ChatID, body)
				if err != nil {
					bot.Send(tgbotapi.NewMessage(ChatID, err.Error()))
				} else {
					bot.Send(tgbotapi.NewMessage(ChatID, "Done!"))
				}
			} else {
				reply := fmt.Sprintf("Unknown command: '%s'", command)
				msg := tgbotapi.NewMessage(ChatID, reply)
				bot.Send(msg)
			}
			log.Printf("[%s] <%d> (%d) %s", UserName, ChatID, UserID, update.Message.Text)
		case <-ticker:
			go checkHealth(ss)
		case oldStatus := <-ss.ChangedState:
			for _, chatID := range dbstore.GetSSChats() {
				log.Printf("Sending to chat %d", chatID)
				msgText := fmt.Sprintf("'%s'\n=>\n'%s'", oldStatus, ss.Status)
				msg := tgbotapi.NewMessage(chatID, msgText)
				bot.Send(msg)
			}
		}
	}
}
