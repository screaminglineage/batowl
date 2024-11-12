package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
		"Get-WmiObject",
		"-Query", "\"SELECT EstimatedChargeRemaining FROM Win32Battery\"")
	batteryLvl, err := cmd.Output()
	if err != nil {
		log.Fatalln("Failed to get battery level: ", err)
	}

	lvl, err := strconv.Atoi(string(batteryLvl))
	if err != nil {
		log.Fatalln("Failed to get battery level: ", err)
	}
	return lvl
}

// TODO: check if the command really works on windows
// TODO: add secondary thread which checks for a keypress to stop recording battery
// TODO: customize the checking intervals

func main() {
	BatLvls, times := traceBattery(1, time.Second)
	fmt.Println(times)
	plotBattery(BatLvls, times)
}

func traceBattery(interval time.Duration, unit time.Duration) ([]int, []int) {
	batteryLvls := make([]int, 1)
	times := make([]int, 1)
	batteryLvls[0] = batteryLvlLinux()
	times[0] = 0

	for prev := 0; prev <= 10; prev++ {
		prevTime := time.Now()
		time.Sleep(interval * unit)
		batLvl := batteryLvlLinux()
		fmt.Printf("Battery Remaining: %d%%\n", batLvl)
		times = append(times, times[prev]+int(time.Now().Sub(prevTime).Seconds()))
		batteryLvls = append(batteryLvls, batLvl)
	}
	return batteryLvls, times
}

func plotBattery(batteryLvls []int, durations []int) {
	p := plot.New()
	p.Title.Text = "laptop charge"
	p.X.Label.Text = "Time"
	p.Y.Label.Text = "Battery Percentage"
	points := makePoints(batteryLvls, durations)
	err := plotutil.AddLinePoints(p,
		"Battery Over time", points)
	if err != nil {
		log.Fatal(err)
	}

	if err := p.Save(10*vg.Inch, 10*vg.Inch, "points.png"); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Successfully generated points.png")
}

func makePoints(batteryLvls []int, durations []int) plotter.XYs {
	n := len(batteryLvls)
	if n != len(durations) {
		panic("There must be an equal number of values for battery levels and duration")
	}

	points := make(plotter.XYs, n)
	for i := range n {
		points[i].X = float64(durations[i])
		points[i].Y = float64(batteryLvls[i])
	}
	return points
}
