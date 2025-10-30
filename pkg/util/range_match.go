package util

import (
	"regexp"
	"strconv"
	"strings"
)

// Package rangematch 提供简单的“区间+通配符”匹配。
// 语法：
//   - *      匹配 [A-Za-z0-9_]+  （至少一个字母/数字/下划线）
//   - [a-b]  整数闭区间，生成 (a|a+1|...|b)

type RangeMatcher struct{ re *regexp.Regexp }

func NewRangeMatcher(pattern string) (*RangeMatcher, error) {
	re, err := parseToRegex(pattern)
	if err != nil {
		return nil, err
	}
	return &RangeMatcher{re: re}, nil
}

func (m *RangeMatcher) Match(input string) bool { return m.re.MatchString(input) }

func parseToRegex(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	for i := 0; i < len(pattern); {
		switch pattern[i] {
		case '*':
			// *** 核心：匹配字母/数字/下划线，至少一个 ***
			b.WriteString(`[A-Za-z0-9_]+`)
			i++
		case '[':
			j := i + 1 + strings.IndexByte(pattern[i+1:], ']')
			if j < i+1 {
				return nil, &parseErr{i, "missing closing ']'"}
			}
			start, end, err := parseRange(pattern[i+1 : j])
			if err != nil {
				return nil, err
			}
			var nums []string
			for k := start; k <= end; k++ {
				nums = append(nums, strconv.Itoa(k))
			}
			b.WriteString(`(` + strings.Join(nums, "|") + `)`)
			i = j + 1
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
			i++
		}
	}
	return regexp.Compile(b.String())
}

func parseRange(s string) (int, int, error) {
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return 0, 0, &parseErr{-1, "bad range format"}
	}
	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, err
	}
	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, err
	}
	if start > end {
		start, end = end, start
	}
	return start, end, nil
}

type parseErr struct {
	pos int
	msg string
}

func (e *parseErr) Error() string { return e.msg }
