// Package appid interpreta AppIDs escritos a mano o como URL de Steam.
package appid

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const maxAppID = 1<<31 - 1

var appIDFromURL = regexp.MustCompile(`(?:store\.steampowered\.com/app/|steamcommunity\.com/app/|steam://(?:store|run|install)/)(\d+)`)

// Parse acepta "49520", "https://store.steampowered.com/app/49520/...",
// "https://steamcommunity.com/app/49520" o "steam://run/49520".
func Parse(input string) (int, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return 0, errors.New("escribe un AppID o una URL de Steam")
	}

	if numeric, err := strconv.Atoi(value); err == nil {
		return validate(numeric)
	}

	if match := appIDFromURL.FindStringSubmatch(value); match != nil {
		numeric, err := strconv.Atoi(match[1])
		if err != nil {
			return 0, fmt.Errorf("AppID inválido en %q", value)
		}
		return validate(numeric)
	}

	return 0, fmt.Errorf("no se reconoce %q como AppID ni como URL de Steam", input)
}

func validate(id int) (int, error) {
	if id <= 0 || id > maxAppID {
		return 0, fmt.Errorf("AppID fuera de rango: %d", id)
	}
	return id, nil
}
