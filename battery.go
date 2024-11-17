package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
