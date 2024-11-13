package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
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

// TODO: add secondary thread which checks for a keypress to stop recording battery
// TODO: take input to set the interval and other options

func main() {
	var batteryLvlFunc func() int
	if runtime.GOOS == "windows" {
		batteryLvlFunc = batteryLvlWindows
	} else {
		batteryLvlFunc = batteryLvlLinux
	}
	batLvls, times := traceBattery(1, time.Second, batteryLvlFunc)
	fmt.Println(times)
	plotBattery(batLvls, times)
}

func traceBattery(interval time.Duration, unit time.Duration, batteryLvl func() int) ([]int, []int) {
	batteryLvls := make([]int, 1)
	times := make([]int, 1)
	// batteryLvls[0] = batteryLvlLinux()
	batteryLvls[0] = batteryLvl()
	times[0] = 0

	for prev := 0; prev <= 3; prev++ {
		prevTime := time.Now()
		time.Sleep(interval * unit)
		batLvl := batteryLvl()
		fmt.Printf("Battery Remaining: %d%%\n", batLvl)

		t := time.Now().Sub(prevTime)
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
	}
	return batteryLvls, times
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
