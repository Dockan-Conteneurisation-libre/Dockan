package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The landing page carries its Esperanto text in the HTML and the other
// languages in an embedded JSON dictionary. Every translatable key of the page
// must exist, non-empty, in every language, and the dictionaries must not
// carry stale keys.
func TestSiteLandingPageTranslationsAreComplete(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "docs", "index.html"))
	if err != nil {
		t.Fatalf("read landing page: %v", err)
	}
	page := string(data)

	keys := map[string]bool{"ui.copied": true, "ui.selected": true}
	attr := regexp.MustCompile(`data-i18n(?:-html|-aria)?="([^"]+)"`)
	for _, m := range attr.FindAllStringSubmatch(page, -1) {
		keys[m[1]] = true
	}
	if len(keys) < 50 {
		t.Fatalf("only %d translatable keys found: page structure changed?", len(keys))
	}

	block := regexp.MustCompile(`(?s)<script type="application/json" id="i18n-data">(.*?)</script>`).FindStringSubmatch(page)
	if block == nil {
		t.Fatal("i18n-data dictionary not found")
	}
	var dict map[string]map[string]string
	if err := json.Unmarshal([]byte(block[1]), &dict); err != nil {
		t.Fatalf("i18n-data is not valid JSON: %v", err)
	}

	for _, lang := range []string{"fr", "en", "es"} {
		entries, ok := dict[lang]
		if !ok {
			t.Errorf("language %s missing", lang)
			continue
		}
		var missing, stale, empty []string
		for k := range keys {
			v, found := entries[k]
			if !found {
				missing = append(missing, k)
			} else if strings.TrimSpace(v) == "" {
				empty = append(empty, k)
			}
		}
		for k := range entries {
			if !keys[k] {
				stale = append(stale, k)
			}
		}
		sort.Strings(missing)
		sort.Strings(stale)
		sort.Strings(empty)
		if len(missing)+len(stale)+len(empty) > 0 {
			t.Errorf("%s: missing %v, stale %v, empty %v", lang, missing, stale, empty)
		}
	}
}
