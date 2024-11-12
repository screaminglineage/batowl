package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"golang.org/x/term"
)

type Message int

const (
	QuitMessage    Message = iota
	RefreshMessage         = iota
)

func main() {
	// Channel to signal the input handling completion
	done := make(chan bool)
	comms := make(chan Message)

	// Goroutine to wait for user input
	go func() {
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			log.Fatalln(err)
		}
		defer term.Restore(int(os.Stdin.Fd()), oldState)

		fmt.Println("Press `q` to quit\r")
		buf := make([]byte, 1)
		for {
			_, err := os.Stdin.Read(buf)
			if err != nil {
				fmt.Println(err, "\r")
				continue
			}

			switch buf[0] {
			case 'q':
				comms <- QuitMessage
				return
			case 'r':
				comms <- RefreshMessage
			}
		}
	}()

	// Goroutine for background work that runs every n seconds
	go func() {
		i := 0
		ticker := time.NewTicker(2 * time.Second) // Change interval as needed
		defer ticker.Stop()
		for {
			select {
			case a := <-comms:
				switch a {
				case QuitMessage:
					fmt.Println("Stopping background work...")
					done <- true
					return // Exit the Goroutine
				case RefreshMessage:
					fmt.Println("Doing background work...", i, "\r")
					i++
				}
			case <-ticker.C:
				fmt.Println("Doing background work...", i, "\r")
				i++
			}
		}
	}()

	// Wait for the input handling Goroutine to finish
	<-done
	fmt.Println("Program stopped.")
}
