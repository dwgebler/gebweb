package main

import (
	"fmt"
	"strings"
)

func workerModuleArguments(args []string) ([]string, bool, error) {
	entry := ""
	var forwarded []string
	for i := 0; i < len(args); i++ {
		if args[i] != "--entry" {
			forwarded = append(forwarded, args[i])
			continue
		}
		if entry != "" || i+1 == len(args) || strings.HasPrefix(args[i+1], "--") {
			return nil, false, fmt.Errorf("gebweb worker: --entry requires one file or module")
		}
		i++
		entry = args[i]
	}
	if entry == "" || strings.HasSuffix(entry, ".gb") {
		return nil, false, nil
	}
	return append([]string{"worker", "--entry", entry}, forwarded...), true, nil
}
