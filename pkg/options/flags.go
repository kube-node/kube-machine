package options

import (
	"strconv"
	"strings"
)

type StringMapOptions struct {
	options map[string]string
}

func New(m map[string]string) StringMapOptions {
	return StringMapOptions{options: m}
}

func (n StringMapOptions) String(key string) string {
	return n.options[key]
}

func (n StringMapOptions) StringSlice(key string) []string {
	a := n.options[key]
	if a == "" {
		return []string{}
	}

	return strings.Split(a, ",")
}

func (n StringMapOptions) Int(key string) int {
	a := n.options[key]
	i, _ := strconv.Atoi(a)
	return i
}

func (n StringMapOptions) Bool(key string) bool {
	b, _ := strconv.ParseBool(n.options[key])
	return b
}

func (n StringMapOptions) Names() []string {
	names := []string{}
	for name, _ := range n.options {
		names = append(names, name)
	}
	return names
}
