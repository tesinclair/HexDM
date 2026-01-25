package main

import (
	"log"
	"os"
	"os/user"
	"errors"
	"fmt"
	"context"
	"os/signal"
	"syscall"

	"github.com/tesinclair/hexdm/pam"
	"github.com/tesinclair/hexdm/tui"
)

var (
	ErrNotRoot = errors.New("HexDM must be run as root.")
)

var (
	username = "blyk" // temporary; will change later
)

func main(){
	log.Println("Starting Hex...")
	
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil{
		if errors.Is(err, context.Canceled){
			log.Printf("Got signal SIGINT. Exiting.")
			os.Exit(0)
		}
		log.Printf("HEX ERR: %v", err)	
		os.Exit(1)
	}

	log.Println("Stopped Hex.")
	os.Exit(0)
}

func run(ctx context.Context) error{
	defer func(){ log.Println("Cleaning Up.") }()

	currUser, err := user.Current()
	if err != nil{
		return fmt.Errorf("Failed to get current user: %w", err)
	}
	if currUser.Uid != "0"{
		return ErrNotRoot
	}

	fprintd, err := pam.ProbeFprintd(username)
	if err != nil{
		log.Printf("Failed to pribe fprintd: %w. Continuing with Password Auth.", err)
		fprintd = false
	}
	if fprintd{
		
	}

	done := make(chan error, 1)
	kill := make(chan bool, 1)

	auth := make(chan error, 1)
	pass := make(chan string, 1)

	go func(){
		done<- tui.Render(kill, auth, pass)
	}()

	
	for{
		select {
		case <-ctx.Done():
			kill<- true
			<-done
			return ctx.Err()
		case err := <-done:
			if err == nil{
				log.Println("Logged in user.")	
				return nil
			}else if errors.Is(err, tui.UserQuit){
				log.Println("User quit session.")
				return nil
			}
			return err
		case password := <-pass:
			ok, err := pam.Authenticate(username, password, false)
			if err != nil{
				log.Printf("Failed to authenticate: %w", err)
				auth<- pam.ErrAuthFailed
			}else if ok == true{
				log.Printf("Authentication Successful. Logging in %v.", username)
				auth<- nil
			}else{
				log.Printf("User %v failed Authentication. Bad Password.", username)
				auth<- errors.New("Invalid Password.")
			}
		}
	}
	return nil
}

