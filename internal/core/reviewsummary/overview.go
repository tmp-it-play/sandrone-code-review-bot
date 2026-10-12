package reviewsummary

import (
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxCharacters = 600
const MaxSentences = 4

var sentenceEnding = regexp.MustCompile(`[.!?。！？](?:\s+|$)`)

type Overview struct {
	Sentences []string `json:"sentences"`
}

func Parse(content string) (string, error) {
	var result Overview
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return "", err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return "", errors.New("요약 JSON 뒤에 추가 내용이 있습니다")
	}
	if len(result.Sentences) < 1 || len(result.Sentences) > MaxSentences {
		return "", errors.New("요약 문장 수가 범위를 벗어났습니다")
	}
	for i, sentence := range result.Sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" || utf8.RuneCountInString(sentence) > 240 || strings.ContainsAny(sentence, "\n\r<>") {
			return "", errors.New("요약 문장이 비어 있거나 너무 길거나 형식이 잘못되었습니다")
		}
		if len(sentenceEnding.FindAllStringIndex(sentence, -1)) > 1 {
			return "", errors.New("요약 항목에 여러 문장이 있습니다")
		}
		last, _ := utf8.DecodeLastRuneInString(sentence)
		if !strings.ContainsRune(".!?。！？", last) {
			sentence += "."
		}
		if utf8.RuneCountInString(sentence) > 240 {
			return "", errors.New("요약 문장이 너무 깁니다")
		}
		result.Sentences[i] = sentence
		for _, prior := range result.Sentences[:i] {
			if similar(sentence, prior) {
				return "", errors.New("요약에서 같은 설명이 반복되었습니다")
			}
		}
	}
	joined := strings.Join(result.Sentences, " ")
	if utf8.RuneCountInString(joined) > MaxCharacters {
		return "", errors.New("최종 요약의 길이가 상한을 초과했습니다")
	}
	return joined, nil
}

func Validate(content string) error {
	_, err := Parse(content)
	return err
}

func similar(left, right string) bool {
	normalize := func(value string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsNumber(r) {
				return unicode.ToLower(r)
			}
			return -1
		}, value)
	}
	a, b := normalize(left), normalize(right)
	if a == b {
		return true
	}
	ar, br := []rune(a), []rune(b)
	if len(ar) >= 36 && len(br) >= 36 && string(ar[:36]) == string(br[:36]) {
		return true
	}
	grams := func(value []rune) map[string]struct{} {
		set := map[string]struct{}{}
		for i := 0; i+3 <= len(value); i++ {
			set[string(value[i:i+3])] = struct{}{}
		}
		return set
	}
	x, y := grams(ar), grams(br)
	intersection := 0
	for gram := range x {
		if _, exists := y[gram]; exists {
			intersection++
		}
	}
	union := len(x) + len(y) - intersection
	return union > 0 && intersection*100 >= union*70
}
