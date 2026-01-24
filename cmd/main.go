package main

import (
	"log"

	"github.com/tesinclair/display-greeter/session"
)

func main(){
	if s, err := session.StartSession(); err != nil{
		log.Fatalf("Failed to request dbus to start session: %w\n", err)	
	}
	defer s.DestroySession()

	s.RunSession()
}
