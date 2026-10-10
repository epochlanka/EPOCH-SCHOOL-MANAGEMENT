package main

import (
	"strings"
	"testing"
)

// Every catalogue message must format cleanly in all three languages.
func TestCatalogueFormats(t *testing.T) {
	args := []any{"A", "B", "C", "D", "E", "F", "G"}
	for key, m := range msgCatalog {
		want := strings.Count(m[0], "%") - 2*strings.Count(m[0], "%%")
		for i, lang := range languages {
			if m[i] == "" {
				t.Errorf("%s: missing %s", key, lang)
				continue
			}
			out := L(lang, key, args[:want]...)
			if strings.Contains(out, "%!") {
				t.Errorf("%s (%s): bad format: %s", key, lang, out)
			}
		}
	}
}
