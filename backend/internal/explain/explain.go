// Package explain — пояснения к вопросам из bank/explanations.json
// (собираются tools/explain/merge.py; цитаты там уже проверены по тексту правил).
package explain

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Ref — ссылка на пункт правил. Ru — дословный перевод drv.am или, если
// нужного места в переводе нет, перевод модели (RuSource = "model").
// Армянский оригинал (hy) в файле есть, но на устройство не отдаётся.
type Ref struct {
	Unit     string `json:"unit"`
	Label    string `json:"label"`
	Ru       string `json:"ru"`
	RuSource string `json:"ru_source"`
}

// Part — пояснение к правильному ответу или к одному неверному варианту.
// Rev меняется при любой правке текста: пометка «некорректно» привязана к Rev.
type Part struct {
	Text string `json:"text"`
	Refs []Ref  `json:"refs"`
	Rev  string `json:"rev"`
}

type Item struct {
	NoBasis bool            `json:"no_basis"` // без опоры на нормативный текст (первая помощь)
	Answer  *Part           `json:"answer,omitempty"`
	Options map[string]Part `json:"options"` // ключ — номер неверного варианта
}

// Part возвращает часть по ключу: "answer" или номер варианта.
func (it Item) Part(key string) (Part, bool) {
	if key == "answer" {
		if it.Answer == nil {
			return Part{}, false
		}
		return *it.Answer, true
	}
	p, ok := it.Options[key]
	return p, ok
}

type Set struct {
	Items map[string]Item `json:"items"`
}

// Load читает файл пояснений. Нет файла — пустой набор: пояснения
// необязательны, тренажёр работает и без них.
func Load(path string) (*Set, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Set{Items: map[string]Item{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var s Set
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("разбор пояснений: %w", err)
	}
	if s.Items == nil {
		s.Items = map[string]Item{}
	}
	return &s, nil
}

func (s *Set) Get(questionID string) (Item, bool) {
	it, ok := s.Items[questionID]
	return it, ok
}
