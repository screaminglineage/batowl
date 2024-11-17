package userinput

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
)

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
			return time.Duration(0), errors.New(fmt.Sprintf("Unsupported unit: `%s`", text))
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
		err = errors.New("Interval cannot be 0 or negative")
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
	// defaults to 0 (no limit)
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
		err = errors.New("Duration cannot be negative")
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

func UserInput() (interval time.Duration, unit time.Duration, duration time.Duration) {
	fmt.Println("Enter interval to record after (eg: 1s/5m/1h) [DEFAULT: 5m]")
	fmt.Print("> ")
	interval, unit, err := parseInterval()
	for err != nil {
		fmt.Println(err, ", Try Again")
		fmt.Print("> ")
		interval, unit, err = parseInterval()
	}
	fmt.Println("Enter duration to stop after, 0 for no limit (eg: 1s/5m/1h) [DEFAULT: 0]")
	fmt.Print("> ")
	duration, err = parseDuration()
	for err != nil {
		fmt.Println(err, ", Try Again")
		fmt.Print("> ")
		duration, err = parseDuration()
	}
	return
}
