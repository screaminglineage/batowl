package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

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

func parseInterval() (interval time.Duration, unit time.Duration, err error) {
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	// defaults to 5 minutes
	if len(text) == 1 && text[0] == '\n' {
		interval = 5
		unit = time.Minute
		return
	}

	i := 0
	for ; unicode.IsDigit(rune(text[i])); i++ {
	}

	num, _ := strconv.Atoi(text[:i])
	if num <= 0 {
		err = errors.New("Interval cannot be 0 or negative,")
		return
	}
	interval = time.Duration(num)

	text = strings.TrimRight(text[i:], "\n")
	switch text {
	case "s":
		unit = time.Second
	case "m":
		unit = time.Minute
	case "h":
		unit = time.Hour
	default:
		err = errors.New(fmt.Sprintf("Unsupported unit: `%s`,", text))
		return
	}
	return
}

var wg sync.WaitGroup

func main() {
	fmt.Println("Enter interval to record after (eg: 1s/5m/1h) [DEFAULT: 5m]")
	fmt.Print("> ")
	interval, unit, err := parseInterval()
	for err != nil {
		fmt.Println(err, "Try Again")
		fmt.Print("> ")
		interval, unit, err = parseInterval()
	}

	var batteryLvlFunc func() int
	if runtime.GOOS == "windows" {
		batteryLvlFunc = batteryLvlWindows
	} else {
		batteryLvlFunc = batteryLvlLinux
	}

	fmt.Printf("Recording Battery every %d second(s)\n", int((interval * unit).Seconds()))

	comms := make(chan Message)
	wg.Add(1)
	go userInput(comms)
	batLvls, times := traceBattery(interval, unit, batteryLvlFunc, comms)
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

func traceBattery(interval time.Duration, unit time.Duration, batteryLvl func() int, comms chan Message) ([]int, []int) {
	update := func(i int, batteryLvls []int, times []int) (newBatteryLvls []int, newTimes []int) {
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
		newBatteryLvls = append(batteryLvls, batteryLvl())
		newTimes = append(times, times[i]+inc)
		return
	}

	batteryLvls := make([]int, 1)
	times := make([]int, 1)
	batteryLvls[0] = batteryLvl()
	times[0] = 0

	ticker := time.NewTicker(interval * unit)
	defer ticker.Stop()

	for prev := 0; ; prev++ {
		select {
		case msg := <-comms:
			switch msg {
			case QuitMessage:
				return batteryLvls, times
				// TODO: the update in case of RefreshMessage does not take into account
				// the time since the last update
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
