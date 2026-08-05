package main

import (
	"reflect"
	"testing"
)

func TestNormalizeProviders(t *testing.T) {
	t.Parallel()

	providers, err := normalizeProviders(" GitHub,aws,github,openai ")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"github", "aws", "openai"}
	if !reflect.DeepEqual(providers, want) {
		t.Fatalf("providers = %v; want %v", providers, want)
	}
}

func TestNormalizeProvidersRejectsInvalidSlug(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"", "bad slug", "-github", "github-"} {
		if _, err := normalizeProviders(value); err == nil {
			t.Errorf("normalizeProviders(%q) unexpectedly succeeded", value)
		}
	}
}
