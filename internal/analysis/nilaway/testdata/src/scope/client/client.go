package client

import (
	"strings"

	"scope/lib"
)

func Safe() int {
	value := 1
	return *lib.Identity(&value)
}

func Unsafe() int {
	return *lib.Missing()
}

func Local() int {
	var p *int
	return *p
}

func StandardLibrary() string {
	return strings.TrimSpace(" safe ")
}
