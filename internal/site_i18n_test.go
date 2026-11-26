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

var (
	i18nAttr   = regexp.MustCompile(`data-i18n(?:-html|-aria)?="([^"]+)"`)
	i18nScript = func(id string) *regexp.Regexp {
		return regexp.MustCompile(`(?s)<script type="application/json" id="` + id + `">(.*?)</script>`)
	}
)

func readI18nBlock(t *testing.T, page, id string) map[string]map[string]string {
	t.Helper()
	m := i18nScript(id).FindStringSubmatch(page)
	if m == nil {
		return nil
	}
	var dict map[string]map[string]string
	if err := json.Unmarshal([]byte(m[1]), &dict); err != nil {
		t.Fatalf("%s is not valid JSON: %v", id, err)
	}
	return dict
}

// Every page of the site carries its Esperanto text in the HTML and the
// other languages in embedded JSON dictionaries (page-specific, plus a
// common one on documentation pages). Every translatable key must exist,
// non-empty, in every language, and the page dictionaries must not carry
// stale keys.
func TestSitePagesTranslationsAreComplete(t *testing.T) {
	pages, err := filepath.Glob(filepath.Join("..", "docs", "*.html"))
	if err != nil {
		t.Fatal(err)
	}
	examples, err := filepath.Glob(filepath.Join("..", "docs", "exemples", "*.html"))
	if err != nil {
		t.Fatal(err)
	}
	pages = append(pages, examples...)
	if len(pages) < 8 {
		t.Fatalf("only %d pages found", len(pages))
	}
	for _, path := range pages {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		checkPageTranslations(t, filepath.Base(path), string(data))
	}
}

func checkPageTranslations(t *testing.T, name, page string) {
	keys := map[string]bool{"ui.copied": true, "ui.selected": true}
	for _, m := range i18nAttr.FindAllStringSubmatch(page, -1) {
		keys[m[1]] = true
	}
	if len(keys) < 12 {
		t.Errorf("%s: only %d translatable keys: page not translated?", name, len(keys))
		return
	}
	dict := readI18nBlock(t, page, "i18n-data")
	if dict == nil {
		t.Errorf("%s: i18n-data dictionary missing", name)
		return
	}
	common := readI18nBlock(t, page, "i18n-common")
	for _, lang := range []string{"fr", "en", "es"} {
		var missing, stale, empty []string
		for k := range keys {
			v, found := dict[lang][k]
			if !found && common != nil {
				v, found = common[lang][k]
			}
			if !found {
				missing = append(missing, k)
			} else if strings.TrimSpace(v) == "" {
				empty = append(empty, k)
			}
		}
		for k := range dict[lang] {
			if !keys[k] {
				stale = append(stale, k)
			}
		}
		sort.Strings(missing)
		sort.Strings(stale)
		sort.Strings(empty)
		if len(missing)+len(stale)+len(empty) > 0 {
			t.Errorf("%s %s: missing %v, stale %v, empty %v", name, lang, missing, stale, empty)
		}
	}
}
