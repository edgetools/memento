package index

import "strings"

// porterStem applies Porter stemming to a lowercase word.
// Words of length ≤ 2 are returned unchanged.
func porterStem(word string) string {
	if len(word) <= 2 {
		return word
	}
	word = porterStep1a(word)
	word = porterStep1b(word)
	word = porterStep1c(word)
	word = porterStep2(word)
	word = porterStep3(word)
	word = porterStep4(word)
	word = porterStep5(word)
	return word
}

// isVowelAt reports whether word[i] is a vowel (a,e,i,o,u or y after a consonant).
func isVowelAt(word string, i int) bool {
	c := word[i]
	if c == 'a' || c == 'e' || c == 'i' || c == 'o' || c == 'u' {
		return true
	}
	if c == 'y' && i > 0 && !isVowelAt(word, i-1) {
		return true
	}
	return false
}

// measure returns the number of VC sequences in stem.
func measure(stem string) int {
	m := 0
	inVowel := false
	for i := 0; i < len(stem); i++ {
		if isVowelAt(stem, i) {
			inVowel = true
		} else if inVowel {
			m++
			inVowel = false
		}
	}
	return m
}

// containsVowel reports whether stem contains at least one vowel.
func containsVowel(stem string) bool {
	for i := 0; i < len(stem); i++ {
		if isVowelAt(stem, i) {
			return true
		}
	}
	return false
}

// endsDoubleConsonant reports whether word ends with two identical consonants.
func endsDoubleConsonant(word string) bool {
	n := len(word)
	if n < 2 {
		return false
	}
	return word[n-1] == word[n-2] && !isVowelAt(word, n-1)
}

// endsCVC reports whether word ends with a consonant-vowel-consonant sequence
// where the final consonant is not w, x, or y.
func endsCVC(word string) bool {
	n := len(word)
	if n < 3 {
		return false
	}
	last := word[n-1]
	if last == 'w' || last == 'x' || last == 'y' {
		return false
	}
	return !isVowelAt(word, n-1) && isVowelAt(word, n-2) && !isVowelAt(word, n-3)
}

// trimSuffix returns (prefix, true) if word ends with suffix, else ("", false).
func trimSuffix(word, suffix string) (string, bool) {
	if strings.HasSuffix(word, suffix) {
		return word[:len(word)-len(suffix)], true
	}
	return "", false
}

func porterStep1a(word string) string {
	if stem, ok := trimSuffix(word, "sses"); ok {
		return stem + "ss"
	}
	if stem, ok := trimSuffix(word, "ies"); ok {
		return stem + "i"
	}
	if _, ok := trimSuffix(word, "ss"); ok {
		return word
	}
	if stem, ok := trimSuffix(word, "s"); ok {
		return stem
	}
	return word
}

func porterStep1b(word string) string {
	if stem, ok := trimSuffix(word, "eed"); ok {
		if measure(stem) > 0 {
			return stem + "ee"
		}
		return word
	}
	fired := false
	rest := word
	if stem, ok := trimSuffix(word, "ed"); ok {
		if containsVowel(stem) {
			rest = stem
			fired = true
		}
	} else if stem, ok := trimSuffix(word, "ing"); ok {
		if containsVowel(stem) {
			rest = stem
			fired = true
		}
	}
	if !fired {
		return word
	}
	if _, ok := trimSuffix(rest, "at"); ok {
		return rest + "e"
	}
	if _, ok := trimSuffix(rest, "bl"); ok {
		return rest + "e"
	}
	if _, ok := trimSuffix(rest, "iz"); ok {
		return rest + "e"
	}
	if endsDoubleConsonant(rest) {
		last := rest[len(rest)-1]
		if last != 'l' && last != 's' && last != 'z' {
			return rest[:len(rest)-1]
		}
	}
	if measure(rest) == 1 && endsCVC(rest) {
		return rest + "e"
	}
	return rest
}

func porterStep1c(word string) string {
	if stem, ok := trimSuffix(word, "y"); ok {
		if containsVowel(stem) {
			return stem + "i"
		}
	}
	return word
}

var step2Rules = [][2]string{
	{"ational", "ate"}, {"tional", "tion"}, {"enci", "ence"},
	{"anci", "ance"}, {"izer", "ize"}, {"abli", "able"},
	{"alli", "al"}, {"entli", "ent"}, {"eli", "e"},
	{"ousli", "ous"}, {"ization", "ize"}, {"ation", "ate"},
	{"ator", "ate"}, {"alism", "al"}, {"iveness", "ive"},
	{"fulness", "ful"}, {"ousness", "ous"}, {"aliti", "al"},
	{"iviti", "ive"}, {"biliti", "ble"},
}

func porterStep2(word string) string {
	for _, rule := range step2Rules {
		if stem, ok := trimSuffix(word, rule[0]); ok {
			if measure(stem) > 0 {
				return stem + rule[1]
			}
			return word
		}
	}
	return word
}

var step3Rules = [][2]string{
	{"icate", "ic"}, {"ative", ""}, {"alize", "al"},
	{"iciti", "ic"}, {"ical", "ic"}, {"ful", ""},
	{"ness", ""},
}

func porterStep3(word string) string {
	for _, rule := range step3Rules {
		if stem, ok := trimSuffix(word, rule[0]); ok {
			if measure(stem) > 0 {
				return stem + rule[1]
			}
			return word
		}
	}
	return word
}

var step4Suffixes = []string{
	"ement", "ment", "ance", "ence", "able", "ible",
	"ism", "ate", "iti", "ous", "ive", "ize",
	"al", "er", "ic", "ion",
}

func porterStep4(word string) string {
	for _, suffix := range step4Suffixes {
		if stem, ok := trimSuffix(word, suffix); ok {
			if suffix == "ion" {
				if measure(stem) > 1 && len(stem) > 0 {
					last := stem[len(stem)-1]
					if last == 's' || last == 't' {
						return stem
					}
				}
				return word
			}
			if measure(stem) > 1 {
				return stem
			}
			return word
		}
	}
	return word
}

func porterStep5(word string) string {
	// Step 5a: remove trailing e
	if stem, ok := trimSuffix(word, "e"); ok {
		m := measure(stem)
		if m > 1 || (m == 1 && !endsCVC(stem)) {
			word = stem
		}
	}
	// Step 5b: ll → l when m > 1
	if endsDoubleConsonant(word) && word[len(word)-1] == 'l' && measure(word) > 1 {
		word = word[:len(word)-1]
	}
	return word
}
