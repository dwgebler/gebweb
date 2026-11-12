package main

import (
	"reflect"
	"testing"
)

func TestWorkerModuleArgumentsRouteOperatorsThroughApplication(t *testing.T) {
	args := []string{"dlq", "retry", "one", "--entry", "app.worker"}

	got, module, err := workerModuleArguments(args)

	want := []string{"worker", "--entry", "app.worker", "dlq", "retry", "one"}
	if err != nil || !module || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q module=%v err=%v", got, module, err)
	}
}

func TestWorkerModuleArgumentsPreserveLegacyFileEntry(t *testing.T) {
	_, module, err := workerModuleArguments([]string{"--entry", "src/main.gb", "--job", "mail"})
	if err != nil || module {
		t.Fatalf("module=%v err=%v", module, err)
	}
}

func TestWorkerModuleArgumentsRejectMissingOrDuplicateEntry(t *testing.T) {
	for _, args := range [][]string{{"--entry"}, {"--entry", "--job"}, {"--entry", "one", "--entry", "two"}} {
		if _, _, err := workerModuleArguments(args); err == nil {
			t.Fatalf("accepted %q", args)
		}
	}
}
