package main

import (
	"log"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestCheckHealth(t *testing.T) {
	ss := &ServerStatus{}
	ss.ChangedState = make(chan string)
	checkHealth(ss)
	log.Printf("mentok")
	t.Logf("OK")
}
