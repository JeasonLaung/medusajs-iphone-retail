package catalog

import (
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type optionDefinitionQuery struct {
	Offset                  int
	Search, Title, ID, Sort string
	Titles, IDs             []string
	Dates                   []definitionDateFilter
	Exclusive               *bool
	WithDeleted             bool
	And, Or                 []optionDefinitionQuery
}

type definitionDateFilter struct {
	Column, Operator string
	Value            time.Time
}

var definitionDateKey = regexp.MustCompile(`^(created_at|updated_at|deleted_at)\[\$(gt|gte|lt|lte|eq|ne)\]$`)
var definitionTimestamp = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,6})?(Z|[+-][0-9]{2}:[0-9]{2})$`)

func parseDefinitionQuery(raw string) (optionDefinitionQuery, error) {
	if len(raw) > 32768 {
		return optionDefinitionQuery{}, errEditInput
	}
	return parseDefinitionGroup(raw, 0, new(int))
}

var definitionGroupKey = regexp.MustCompile(`^\$(and|or)\[(0|[1-9][0-9]*)\]\[([^\]]+)\](\[.*\])?$`)

func parseDefinitionGroup(raw string, depth int, nodes *int) (optionDefinitionQuery, error) {
	*nodes++
	if depth > 4 || *nodes > 64 {
		return optionDefinitionQuery{}, errEditInput
	}
	q := optionDefinitionQuery{}
	values, e := url.ParseQuery(raw)
	if e != nil {
		return q, errEditInput
	}
	if len(raw) > 32768 || len(values) > 100 {
		return q, errEditInput
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	groups := map[string]map[int]url.Values{"and": {}, "or": {}}
	for _, key := range keys {
		vs := values[key]
		if strings.HasPrefix(key, "$and[") || strings.HasPrefix(key, "$or[") {
			m := definitionGroupKey.FindStringSubmatch(key)
			if m == nil {
				return q, errEditInput
			}
			index, err := strconv.Atoi(m[2])
			if err != nil || index > 15 {
				return q, errEditInput
			}
			child := groups[m[1]][index]
			if child == nil {
				child = url.Values{}
				groups[m[1]][index] = child
			}
			child[m[3]+m[4]] = vs
			continue
		}
		if depth > 0 && (key == "sort" || key == "offset" || key == "with_deleted") {
			return q, errEditInput
		}
		if key == "id[]" || key == "title[]" {
			scalar := strings.TrimSuffix(key, "[]")
			if values[scalar] != nil || len(vs) > 100 {
				return q, errEditInput
			}
			seen := map[string]bool{}
			for _, value := range vs {
				if !utf8.ValidString(value) || value == "" || seen[value] {
					return q, errEditInput
				}
				seen[value] = true
				for _, r := range value {
					if unicode.IsControl(r) {
						return q, errEditInput
					}
				}
				if scalar == "id" && (!validProductID(value) || strings.ContainsAny(value, "/\\?#")) || scalar == "title" && utf8.RuneCountInString(value) > 100 {
					return q, errEditInput
				}
			}
			if scalar == "id" {
				q.IDs = vs
			} else {
				q.Titles = vs
			}
			continue
		}
		if len(vs) != 1 || vs[0] == "" || !utf8.ValidString(vs[0]) {
			return q, errEditInput
		}
		value := vs[0]
		for _, r := range value {
			if unicode.IsControl(r) {
				return q, errEditInput
			}
		}
		switch key {
		case "is_exclusive", "with_deleted":
			if value != "true" && value != "false" {
				return q, errEditInput
			}
			b := value == "true"
			if key == "is_exclusive" {
				q.Exclusive = &b
			} else {
				q.WithDeleted = b
			}
		case "offset":
			n, e := strconv.Atoi(value)
			if e != nil || n < 0 || n > 100000 || strconv.Itoa(n) != value {
				return q, errEditInput
			}
			q.Offset = n
		case "q":
			if utf8.RuneCountInString(value) > 200 || strings.TrimSpace(value) == "" {
				return q, errEditInput
			}
			q.Search = strings.TrimSpace(value)
		case "title":
			if utf8.RuneCountInString(value) > 100 {
				return q, errEditInput
			}
			q.Title = value
		case "id":
			if !validProductID(value) || strings.ContainsAny(value, "/\\?#") {
				return q, errEditInput
			}
			q.ID = value
		case "sort":
			switch value {
			case "title_asc", "title_desc", "created_at_asc", "created_at_desc", "updated_at_asc", "updated_at_desc":
				q.Sort = value
			default:
				return q, errEditInput
			}
		default:
			matches := definitionDateKey.FindStringSubmatch(key)
			if matches == nil || !definitionTimestamp.MatchString(value) {
				return q, errEditInput
			}
			date, err := time.Parse(time.RFC3339Nano, value)
			if err != nil || date.Year() < 1000 || date.Year() > 9999 || date.UTC().Year() < 1000 || date.UTC().Year() > 9999 {
				return q, errEditInput
			}
			q.Dates = append(q.Dates, definitionDateFilter{matches[1], matches[2], date.UTC()})
		}
	}
	for _, kind := range []string{"and", "or"} {
		for index := 0; index < len(groups[kind]); index++ {
			child, ok := groups[kind][index]
			if !ok {
				return q, errEditInput
			}
			parsed, err := parseDefinitionGroup(child.Encode(), depth+1, nodes)
			if err != nil {
				return q, errEditInput
			}
			if kind == "and" {
				q.And = append(q.And, parsed)
			} else {
				q.Or = append(q.Or, parsed)
			}
		}
	}
	return q, nil
}
