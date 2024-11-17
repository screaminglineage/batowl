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

var wg sync.WaitGroup
var oldTermState *term.State

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

func readInputAndTrim() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(text, "\r\n"), nil
}

func parseNumber(text string) (int, string, error) {
	i := 0
	for ; i < len(text) && unicode.IsDigit(rune(text[i])); i++ {
	}
	num, err := strconv.Atoi(text[:i])
	return num, text[i:], err
}

func parseUnit(text string) (time.Duration, error) {
	switch text {
	case "s":
		return time.Second, nil
	case "m":
		return time.Minute, nil
	case "h":
		return time.Hour, nil
	default:
		if len(text) == 0 {
			return time.Duration(0), errors.New("Unit must be provided")
		} else {
			return time.Duration(0), errors.New(fmt.Sprintf("Unsupported unit: `%s`,", text))
		}
	}
}

func parseInterval() (interval time.Duration, unit time.Duration, err error) {
	text, err := readInputAndTrim()
	if err != nil {
		return
	}

	// defaults to 5 minutes
	if len(text) == 0 {
		interval = 5
		unit = time.Minute
		return
	}

	num, text, err := parseNumber(text)
	if err != nil {
		err = errors.New("Interval must be a number")
		return
	}
	if num <= 0 {
		err = errors.New("Interval cannot be 0 or negative,")
		return
	}
	interval = time.Duration(num)
	unit, err = parseUnit(text)
	return
}

func parseDuration() (duration time.Duration, err error) {
	text, err := readInputAndTrim()
	if err != nil {
		return
	}
	// defaults to 0 (no duration)
	if len(text) == 0 {
		duration = time.Duration(0)
		return
	}

	n, text, err := parseNumber(text)
	if err != nil {
		err = errors.New("Duration must be a number")
		return
	}
	if n < 0 {
		err = errors.New("Duration cannot be negative,")
		return
	}
	num := time.Duration(n)

	if n == 0 {
		duration = time.Duration(0)
		return
	}
	unit, err := parseUnit(text)
	duration = time.Duration(num * unit)
	return
}

// TODO: try to fix the hacky way in which the "stop after duration" feature is implemented
func main() {
	fmt.Println("Enter interval to record after (eg: 1s/5m/1h) [DEFAULT: 5m]")
	fmt.Print("> ")
	interval, unit, err := parseInterval()
	for err != nil {
		fmt.Println(err, "Try Again")
		fmt.Print("> ")
		interval, unit, err = parseInterval()
	}
	fmt.Println("Enter duration to stop after, 0 for no limit (eg: 1s/5m/1h) [DEFAULT: 0]")
	fmt.Print("> ")
	duration, err := parseDuration()
	for err != nil {
		fmt.Println(err, "Try Again")
		fmt.Print("> ")
		duration, err = parseDuration()
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
	batLvls, times := traceBattery(interval, unit, duration, batteryLvlFunc, comms)
	wg.Wait()
	plotBattery(batLvls, times)
	fmt.Println("Successfully generated points.png")
}

func userInput(comms chan Message) {
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
