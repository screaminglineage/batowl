package main

import (
	"fmt"
	"golang.org/x/term"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

type Message int

const (
	QuitMessage    Message = iota
	RefreshMessage         = iota
)

func batteryLvlLinux() int {
	batteryFilePath, err := filepath.Glob("/sys/class/power_supply/BAT*")
	if err != nil || len(batteryFilePath) == 0 {
		log.Fatal("failed to get battery")
	}

	battery0 := batteryFilePath[0]
	batteryFile, err := os.Open(filepath.Join(battery0, "capacity"))
	if err != nil {
		log.Fatal(err)
	}
	defer batteryFile.Close()

	var lvl int
	_, err = fmt.Fscanf(batteryFile, "%d", &lvl)
	if err != nil {
		log.Fatal(err)
	}
	return lvl
}

func batteryLvlWindows() int {
	cmd := exec.Command(
		"powershell", "-Command",
		"(Get-WmiObject -Query 'SELECT EstimatedChargeRemaining FROM Win32_Battery').EstimatedChargeRemaining")
	batteryLvl, err := cmd.Output()
	if err != nil {
		log.Fatalln("Failed to get battery level: ", err)
	}

	lvl, err := strconv.Atoi(strings.TrimRight(string(batteryLvl), "\r\n"))
	if err != nil {
		log.Fatalln("Failed to get battery level: ", err)
	}
	return lvl
}

// TODO: take input to set the interval and other options

// func run(batteryLvlFunc func() int) {
// 	// Channel to signal the input handling completion
// 	done := make(chan bool)
// 	comms := make(chan Message)
//
// 	// Goroutine to wait for user input
// 	go func() {
// 		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
// 		if err != nil {
// 			log.Fatalln(err)
// 		}
// 		defer term.Restore(int(os.Stdin.Fd()), oldState)
//
// 		fmt.Println("Press `q` to quit\r")
// 		buf := make([]byte, 1)
// 		for {
// 			_, err := os.Stdin.Read(buf)
// 			if err != nil {
// 				fmt.Println(err, "\r")
// 				continue
// 			}
//
// 			switch buf[0] {
// 			case 'q':
// 				comms <- QuitMessage
// 				return
// 			case 'r':
// 				comms <- RefreshMessage
// 			}
// 		}
// 	}()
//
// 	// Goroutine for background work that runs every n seconds
// 	go func() {
// 		i := 0
// 		ticker := time.NewTicker(2 * time.Second) // Change interval as needed
// 		defer ticker.Stop()
// 		for {
// 			select {
// 			case a := <-comms:
// 				switch a {
// 				case QuitMessage:
// 					fmt.Println("Stopping background work...")
// 					done <- true
// 					return // Exit the Goroutine
// 				case RefreshMessage:
// 					fmt.Println("Doing background work...", i, "\r")
// 					i++
// 				}
// 			case <-ticker.C:
// 				batLvls, times := traceBattery(1, time.Second, batteryLvlFunc)
// 				i++
// 			}
// 		}
// 	}()

// Wait for the input handling Goroutine to finish
// 	<-done
// 	fmt.Println("Program stopped.")
// }

var wg sync.WaitGroup

func main() {
	var batteryLvlFunc func() int
	if runtime.GOOS == "windows" {
		batteryLvlFunc = batteryLvlWindows
	} else {
		batteryLvlFunc = batteryLvlLinux
	}

	comms := make(chan Message)

	wg.Add(1)
	go userInput(comms)
	batLvls, times := traceBattery(1, time.Second, batteryLvlFunc, comms)
	wg.Wait()

	fmt.Println(times)
	plotBattery(batLvls, times)
}

func userInput(comms chan Message) {
	defer wg.Done()
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
}

func traceBattery(interval time.Duration, unit time.Duration, batteryLvl func() int, comms chan Message) ([]int, []int) {
	batteryLvls := make([]int, 1)
	times := make([]int, 1)
	batteryLvls[0] = batteryLvl()
	times[0] = 0

	ticker := time.NewTicker(interval * unit)
	defer ticker.Stop()

	// prevTime := time.Now()
	for prev := 0; ; prev++ {
		select {
		case msg := <-comms:
			switch msg {
			case QuitMessage:
				fmt.Println("Quitting...\r")
				return batteryLvls, times
			case RefreshMessage:
				log.Panic("todo")
			}
		case <-ticker.C:
			batLvl := batteryLvl()
			fmt.Printf("Battery Remaining: %d%%\n\r", batLvl)

			// t := time.Now().Sub(prevTime)
			t := interval * unit
			var inc int
			switch unit {
			case time.Second:
				inc = int(t.Seconds())
			case time.Minute:
				inc = int(t.Minutes())
			case time.Hour:
				inc = int(t.Hours())
			default:
				log.Panicln("traceBattery: Unreachable")
			}

			times = append(times, times[prev]+inc)
			batteryLvls = append(batteryLvls, batLvl)
			// prevTime = time.Now()
		}
	}
}

func plotBattery(batteryLvls []int, times []int) {
	p := plot.New()
	p.Title.Text = "Laptop Charge"
	p.X.Label.Text = "Time"
	p.Y.Label.Text = "Battery Percentage"
	points := makePoints(batteryLvls, times)
	err := plotutil.AddLinePoints(p,
		"Battery Over Time", points)
	if err != nil {
		log.Fatal(err)
	}

	if err := p.Save(10*vg.Inch, 10*vg.Inch, "points.png"); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Successfully generated points.png")
}

func makePoints(batteryLvls []int, times []int) plotter.XYs {
	n := len(batteryLvls)
	if n != len(times) {
		panic("There must be an equal number of values for battery levels and duration")
	}

	points := make(plotter.XYs, n)
	for i := range n {
		points[i].X = float64(times[i])
		points[i].Y = float64(batteryLvls[i])
	}
	return points
}
