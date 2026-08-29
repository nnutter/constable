package directive

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/nnutter/constable/internal/directive/enum"
)

const Prefix = "//constable:"

type Check struct {
	Name   string
	Config string
}

func Parse(comment string) ([]Check, error) {
	comment = strings.TrimSpace(comment)
	if !strings.HasPrefix(comment, Prefix) {
		return nil, nil
	}

	text := strings.TrimSpace(strings.TrimPrefix(comment, Prefix))
	if text == "" {
		return nil, fmt.Errorf("no check tag after %s", Prefix)
	}
	tag := reflect.StructTag(text)

	var checks []Check
	for _, key := range keys(tag) {
		switch enum.ParseCheckType(key).(type) {
		case enum.Nonmutating:
			checks = append(checks, Check{Name: key, Config: tag.Get(key)})
			continue
		}
		return nil, fmt.Errorf("invalid tag near %q", text)
	}

	return checks, nil
}

func HasNonmutating(comment string) bool {
	checks, err := Parse(comment)
	if err != nil {
		return false
	}

	for _, check := range checks {
		if check.Name == enum.NonmutatingName {
			return true
		}
	}

	return false
}

func keys(tag reflect.StructTag) []string {
	keyValuePairsPattern := regexp.MustCompile(`(?P<key>\w+)(?::"[^"]*")?`)
	keyValuePairs := keyValuePairsPattern.FindAllStringSubmatch(string(tag), -1)
	keyIndex := keyValuePairsPattern.SubexpIndex("key")
	if keyIndex == -1 {
		panic("key index not found")
	}

	keys := make([]string, 0, len(keyValuePairs))
	for _, keyValuePair := range keyValuePairs {
		keys = append(keys, keyValuePair[keyIndex])
	}
	return keys
}
