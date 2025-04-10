package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"
	"time"

	"github.com/pterm/pterm"
)

func ContainsAny(line string, substrings ...string) bool {
	for _, s := range substrings {
		if strings.Contains((line), s) {
			return true
		}
	}
	return false
}

func PrintLogLine(podName string, line string, keyword string, keywordOnly bool, showPodName bool) {
	var logEntry map[string]interface{}
	var colorFunc func(a ...interface{}) string
	var timestamp string

	if timestampFlag {
		// Extract timestamp and rest of the line
		if parts := strings.SplitN(line, " ", 2); len(parts) == 2 {
			timestamp = parts[0]
			line = parts[1]
		}
	}
	switch {
	case IsError(line):
		colorFunc = pterm.Red
	case IsWarning(line):
		colorFunc = pterm.Yellow
	case IsPanic(line):
		colorFunc = pterm.Yellow
	case IsDebug(line):
		colorFunc = pterm.Cyan
	default:
		colorFunc = pterm.White
	}

	if err := json.Unmarshal([]byte(line), &logEntry); err == nil {
		level, exists := logEntry["level"].(string)
		if exists {
			levelLower := strings.ToLower(level)
			switch {
			case ContainsAny(levelLower, strings.Split(errorLevelJson, "|")...):
				colorFunc = pterm.Red
			case ContainsAny(levelLower, strings.Split(warnLevelJson, "|")...):
				colorFunc = pterm.Yellow
			case ContainsAny(levelLower, strings.Split(debugLevelJson, "|")...):
				colorFunc = pterm.Cyan
			default:
				colorFunc = pterm.White
			}
		}
	}

	// Convert timestamp string to time.Time object
	if timestamp != "" {
		t, err := time.Parse(time.RFC3339Nano, timestamp)
		if err == nil {
			timestamp = t.Format(timestampFormat)
		}
	}

	var prefix string
	if showPodName {
		prefix = fmt.Sprintf("[%s] ", podName)
	}

	if keyword != "" && keywordOnly {
		// Only show lines that contain the keyword
		if strings.Contains(line, keyword) {
			coloredLine := HighlightKeyword(colorFunc(line), keyword, colorFunc)
			fmt.Printf("%s%s %s\n", prefix, pterm.FgDarkGray.Sprint(timestamp), coloredLine)
		}
	} else if keyword != "" {
		coloredLine := HighlightKeyword(colorFunc(line), keyword, colorFunc)
		fmt.Printf("%s%s %s\n", prefix, pterm.FgDarkGray.Sprint(timestamp), coloredLine)
	} else {
		fmt.Printf("%s%s %s\n", prefix, pterm.FgDarkGray.Sprint(timestamp), colorFunc(line))
	}
}

func GetPodColor(podName string) pterm.Color {
	// Calculer le hachage du nom du pod
	hash := fnv.New32a()
	hash.Write([]byte(podName))
	hashValue := hash.Sum32()

	// Utiliser le hachage pour choisir une couleur distincte dans la palette
	colorIndex := int(hashValue) % len(colorPalette)
	return colorPalette[colorIndex]
}

func HighlightKeyword(line string, keyword string, colorFunc func(a ...interface{}) string) string {
	re := regexp.MustCompile(keyword)
	matches := re.FindAllStringIndex(line, -1)

	if len(matches) > 0 {
		result := ""
		startIndex := 0
		for _, match := range matches {
			result += colorFunc(line[startIndex:match[0]]) + pterm.BgMagenta.Sprint(line[match[0]:match[1]])
			startIndex = match[1]
		}
		result += colorFunc(line[startIndex:])
		return result
	}

	return colorFunc(line)
}
