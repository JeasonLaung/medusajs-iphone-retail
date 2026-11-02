package catalog

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/blevesearch/snowballstem"
	"github.com/blevesearch/snowballstem/english"
)

// Ordered token recognition follows the source English parser's overlapping
// URL/host/path and compound lexemes. Locale and further malformed-input
// boundaries still require differential acceptance against PostgreSQL.
const searchHost = `[a-z0-9](?:[a-z0-9_-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9_-]*[a-z0-9])?)+(?::[0-9]+)?`
const searchURLPath = `/[^\x00-\x20\x{7f}-\x{10ffff}"<>\\^\x60{|}]+`

var searchEnglishTokens = regexp.MustCompile(
	`(?P<skip>(?s:<!--.*?-->|<\?x[^>]*>|</[a-z_][a-z0-9_.:-]*(?:\s[^>]*)?>|<[a-z_:][a-z0-9_.:-]*(?:[^>"']|"[^"]*"|'[^']*')*>|&(?:#[0-9]+|#x[0-9a-f]+|[a-z][a-z0-9]*);))` +
		`|(?P<protocol>[a-z]+://)` +
		`|(?P<email>[a-z0-9][a-z0-9_.-]*@` + searchHost + `)` +
		`|(?P<scientific>[+-]?[0-9]+(?:\.[0-9]+)?e[+-]?[0-9]+)` +
		`|(?P<version>[0-9]+(?:\.[0-9]+){2,})` +
		`|(?P<decimal>[+-]?[0-9]+\.[0-9]+)` +
		`|(?P<link>` + searchHost + `(?:` + searchURLPath + `)?)` +
		`|(?P<file>(?:\.{1,2}/|/|[a-z0-9_][a-z0-9_.-]*/)[a-z0-9_.~][a-z0-9_./~-]*)` +
		`|(?P<compound>[\pL][\pL\pN]*(?:-[\pL\pN]+)+)` +
		`|(?P<word>[\pL\pN]+)` +
		`|(?P<integer>[+-][0-9]+)`)
var searchIgnoredTag = regexp.MustCompile(`^<(\/?)(script|style)(?:\s|>)`)

// PostgreSQL English dictionary stopwords, applied before stemming.
// https://github.com/postgres/postgres/blob/REL_17_STABLE/src/backend/snowball/stopwords/english.stop
var searchEnglishStopwords = func() map[string]bool {
	words := strings.Fields(`i me my myself we our ours ourselves you your yours yourself yourselves
he him his himself she her hers herself it its itself they them their theirs themselves
what which who whom this that these those am is are was were be been being
have has had having do does did doing a an the and but if or because as until while
of at by for with about against between into through during before after above below
to from up down in out on off over under again further then once here there when where why how
all any both each few more most other some such no nor not only own same so than too very
s t can will just don should now`)
	result := make(map[string]bool, len(words))
	for _, word := range words {
		result[word] = true
	}
	return result
}()

type searchLexeme struct {
	Term     string `json:"term"`
	Position int    `json:"position"`
}

func searchTerms(s string) []string {
	lexemes := searchLexemes(s)
	terms := make([]string, 0, len(lexemes))
	for _, lexeme := range lexemes {
		terms = append(terms, lexeme.Term)
	}
	return terms
}

// Dictionary stopwords consume positions even though they produce no lexeme.
// Retaining those gaps is necessary for source cover-density relevance.
func searchLexemes(s string) []searchLexeme {
	tokens := searchEnglishTokens.FindAllStringSubmatch(strings.ToLower(s), -1)
	terms := make([]searchLexeme, 0, len(tokens))
	position := 0
	appendTerm := func(term string) {
		position++
		terms = append(terms, searchLexeme{term, position})
	}
	// Environments are mutable; keep one local to this call for concurrent searches.
	env := snowballstem.NewEnv("")
	appendWord := func(word string) {
		if searchEnglishStopwords[word] {
			position++
			return
		}
		if strings.IndexFunc(word, unicode.IsNumber) >= 0 {
			appendTerm(word)
			return
		}
		env.SetCurrent(word)
		english.Stem(env)
		if term := env.Current(); term != "" {
			appendTerm(term)
		}
	}
	ignore := false
	for _, token := range tokens {
		kind := ""
		for i, name := range searchEnglishTokens.SubexpNames() {
			if i > 0 && token[i] != "" {
				kind = name
				break
			}
		}
		word := token[0]
		if kind == "skip" {
			if tag := searchIgnoredTag.FindStringSubmatch(word); tag != nil {
				ignore = tag[1] == ""
			}
			continue
		}
		if ignore || kind == "protocol" {
			continue
		}
		switch kind {
		case "email", "scientific", "version", "decimal", "integer", "file":
			appendTerm(word)
		case "link":
			if slash := strings.IndexByte(word, '/'); slash >= 0 {
				appendTerm(word)
				appendTerm(word[:slash])
				appendTerm(word[slash:])
			} else {
				appendTerm(word)
			}
		default:
			appendWord(word)
			if kind == "compound" {
				for _, part := range strings.Split(word, "-") {
					appendWord(part)
				}
			}
		}
	}
	return terms
}
