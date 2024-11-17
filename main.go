package main

import (
	"batowl/userinput"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	"golang.org/x/term"

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

var wg sync.WaitGroup
var oldTermState *term.State

func main() {
	interval, unit, duration := userinput.UserInput()

	var batteryLvlFunc func() int
	if runtime.GOOS == "windows" {
		batteryLvlFunc = batteryLvlWindows
	} else {
		batteryLvlFunc = batteryLvlLinux
	}

	fmt.Printf("Recording Battery every %d second(s)\n", int((interval * unit).Seconds()))

	comms := make(chan Message)
	wg.Add(1)
	go recordingControl(comms)
	batLvls, times := traceBattery(interval, unit, duration, batteryLvlFunc, comms)
	wg.Wait()
	plotBattery(batLvls, times)
	fmt.Println("Successfully generated points.png")
}

func recordingControl(comms chan Message) {
	defer wg.Done()
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	oldTermState = oldState

	if err != nil {
		log.Fatalln(err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldTermState)

	fmt.Println("Press `q` to quit, `r` to force a recording now\r")
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

func traceBattery(
	interval time.Duration,
	unit time.Duration,
	duration time.Duration,
	batteryLvl func() int,
	comms chan Message) ([]int, []int) {

	batteryLvls := make([]int, 1)
	times := make([]int, 1)
	batteryLvls[0] = batteryLvl()
	times[0] = 0

	prevTime := time.Now()
	update := func(i int, batteryLvls []int, times []int) (newBatteryLvls []int, newTimes []int) {
		elapsed := time.Now().Sub(prevTime)
		prevTime = time.Now()
		var inc int
		switch unit {
		case time.Second:
			inc = int(elapsed.Seconds())
		case time.Minute:
			inc = int(elapsed.Minutes())
		case time.Hour:
			inc = int(elapsed.Hours())
		default:
			log.Panicln("traceBattery: Unreachable")
		}
		newBatteryLvls = append(batteryLvls, batteryLvl())
		newTimes = append(times, times[i]+inc)
		return
	}

	// TODO: try to fix the hacky way in which the "stop after duration" feature is implemented
	if duration != 0 {
		go func() {
			time.Sleep(duration)
			comms <- QuitMessage
			term.Restore(int(os.Stdin.Fd()), oldTermState)
			wg.Done()
		}()
	}

	ticker := time.NewTicker(interval * unit)
	defer ticker.Stop()

	for prev := 0; ; prev++ {
		select {
		case msg := <-comms:
			switch msg {
			case QuitMessage:
				return batteryLvls, times
			case RefreshMessage:
				batteryLvls, times = update(prev, batteryLvls, times)
				fmt.Printf("Updated Record [%d]\r", prev)
			}
		case <-ticker.C:
			batteryLvls, times = update(prev, batteryLvls, times)
		}
	}
}

func plotBattery(batteryLvls []int, times []int) {
	p := plot.New()
	p.Title.Text = "Battery Charge vs Time"
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
