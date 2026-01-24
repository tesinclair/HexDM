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

	// "github.com/tesinclair/hexdm/pam"
	"github.com/tesinclair/hexdm/tui"
)

var(
	ErrNotRoot = errors.New("HexDM must be run as root.")
)

func main(){
	log.Println("Starting Hex...")
	
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil{
		if err == context.Canceled{
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

	user, err := user.Current()
	if err != nil{
		return fmt.Errorf("Failed to get current user: %w", err)
	}
	if user.Uid != "0"{
		return ErrNotRoot
	}

	done := make(chan error, 1)
	kill := make(chan bool, 1)
	go func(){
		done<- tui.Render(kill)
	}()

	select {
	case <-ctx.Done():
		kill<- true
		<-done
		return ctx.Err()
	case err := <-done:
		return err
	}
	return nil
}

