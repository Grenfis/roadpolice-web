// Package bank хранит банк экзаменационных вопросов и собирает из него билеты.
package bank

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
)

// Question — вопрос банка. Answer — номер правильного варианта (1-based);
// в экзаменационном режиме он обнуляется перед отправкой во фронтенд.
type Question struct {
	ID      string   `json:"id"`
	Group   int      `json:"group"`
	Number  int      `json:"number"` // номер внутри группы, как в харцашаре
	Section string   `json:"section"`
	Text    string   `json:"text"`
	Image   string   `json:"image"`
	Options []string `json:"options"`
	Answer  int      `json:"answer"`
}

type Section struct {
	Title     string `json:"title"`
	PerTicket int    `json:"per_ticket"`
}

type ExamRules struct {
	Questions      int `json:"questions"`
	TimeMinutes    int `json:"time_minutes"`
	PassMinCorrect int `json:"pass_min_correct"`
}

type Meta struct {
	Category string             `json:"category"`
	Language string             `json:"language"`
	Source   string             `json:"source"`
	Total    int                `json:"total"`
	Sections map[string]Section `json:"sections"`
	Exam     ExamRules          `json:"exam"`
	Groups   []int              `json:"groups"`
}

type Bank struct {
	Meta      Meta        `json:"meta"`
	Questions []*Question `json:"questions"`

	byID       map[string]*Question
	bySection  map[string][]*Question
	byGroup    map[int][]*Question
	sectionIDs []string // разделы в порядке их следования в постановлении
}

// Load разбирает bank.json и строит индексы.
func Load(data []byte) (*Bank, error) {
	var b Bank
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("разбор банка: %w", err)
	}
	if len(b.Questions) == 0 {
		return nil, fmt.Errorf("банк пуст")
	}
	b.byID = make(map[string]*Question, len(b.Questions))
	b.bySection = make(map[string][]*Question)
	b.byGroup = make(map[int][]*Question)
	for _, q := range b.Questions {
		if _, dup := b.byID[q.ID]; dup {
			return nil, fmt.Errorf("дубль id %s", q.ID)
		}
		if q.Answer < 1 || q.Answer > len(q.Options) {
			return nil, fmt.Errorf("%s: ответ %d вне вариантов (%d)", q.ID, q.Answer, len(q.Options))
		}
		b.byID[q.ID] = q
		b.bySection[q.Section] = append(b.bySection[q.Section], q)
		b.byGroup[q.Group] = append(b.byGroup[q.Group], q)
	}
	for s := range b.Meta.Sections {
		if len(b.bySection[s]) == 0 {
			return nil, fmt.Errorf("раздел %s объявлен в meta, но вопросов нет", s)
		}
	}
	// порядок разделов: как в п.17 постановления
	for _, s := range []string{"law", "pdd", "faults", "first_aid"} {
		if _, ok := b.Meta.Sections[s]; ok {
			b.sectionIDs = append(b.sectionIDs, s)
		}
	}
	for s := range b.Meta.Sections {
		if !contains(b.sectionIDs, s) {
			b.sectionIDs = append(b.sectionIDs, s)
		}
	}
	return &b, nil
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func (b *Bank) ByID(id string) (*Question, bool) {
	q, ok := b.byID[id]
	return q, ok
}

func (b *Bank) SectionIDs() []string { return append([]string(nil), b.sectionIDs...) }

func (b *Bank) CountBySection() map[string]int {
	out := make(map[string]int, len(b.bySection))
	for s, qs := range b.bySection {
		out[s] = len(qs)
	}
	return out
}

func (b *Bank) CountByGroup() map[int]int {
	out := make(map[int]int, len(b.byGroup))
	for g, qs := range b.byGroup {
		out[g] = len(qs)
	}
	return out
}

// Ticket собирает экзаменационный билет: из каждого раздела берётся столько
// вопросов, сколько предписывает п.17 Порядка (2 закон + 15 ПДД + 2
// неисправности + 1 первая помощь = 20), выбор внутри раздела случайный.
func (b *Bank) Ticket(rnd *rand.Rand) ([]*Question, error) {
	var out []*Question
	for _, sid := range b.sectionIDs {
		need := b.Meta.Sections[sid].PerTicket
		pool := b.bySection[sid]
		if need > len(pool) {
			return nil, fmt.Errorf("раздел %s: нужно %d вопросов, в банке %d", sid, need, len(pool))
		}
		for _, i := range rnd.Perm(len(pool))[:need] {
			out = append(out, pool[i])
		}
	}
	if want := b.Meta.Exam.Questions; want > 0 && len(out) != want {
		return nil, fmt.Errorf("сумма квот %d не равна размеру билета %d", len(out), want)
	}
	return out, nil
}

// Selection описывает выборку вопросов для режима тренировки.
type Selection struct {
	Scope   string   `json:"scope"`   // "all" | "group" | "section" | "ids"
	Group   int      `json:"group"`   // для scope=group
	Section string   `json:"section"` // для scope=section
	IDs     []string `json:"ids"`     // для scope=ids (работа над ошибками)
	Shuffle bool     `json:"shuffle"`
}

// Select возвращает вопросы для тренировки. Без перемешивания порядок всегда
// один и тот же: по группе, затем по номеру вопроса внутри группы.
func (b *Bank) Select(sel Selection, rnd *rand.Rand) ([]*Question, error) {
	var qs []*Question
	switch sel.Scope {
	case "", "all":
		qs = append(qs, b.Questions...)
	case "group":
		qs = append(qs, b.byGroup[sel.Group]...)
		if len(qs) == 0 {
			return nil, fmt.Errorf("нет группы %d", sel.Group)
		}
	case "section":
		qs = append(qs, b.bySection[sel.Section]...)
		if len(qs) == 0 {
			return nil, fmt.Errorf("нет раздела %q", sel.Section)
		}
	case "ids":
		for _, id := range sel.IDs {
			if q, ok := b.byID[id]; ok {
				qs = append(qs, q)
			}
		}
	default:
		return nil, fmt.Errorf("неизвестная выборка %q", sel.Scope)
	}
	if sel.Scope == "ids" {
		// порядок как передали (список ошибок уже отсортирован)
		if sel.Shuffle {
			shuffle(qs, rnd)
		}
		return qs, nil
	}
	// порядок как в харцашаре: по группе, внутри группы по номеру вопроса
	sort.SliceStable(qs, func(i, j int) bool {
		if qs[i].Group != qs[j].Group {
			return qs[i].Group < qs[j].Group
		}
		return qs[i].Number < qs[j].Number
	})
	if sel.Shuffle {
		shuffle(qs, rnd)
	}
	return qs, nil
}

func shuffle(qs []*Question, rnd *rand.Rand) {
	rnd.Shuffle(len(qs), func(i, j int) { qs[i], qs[j] = qs[j], qs[i] })
}

// WithoutAnswers копирует вопросы, убирая правильный ответ — для экзамена,
// где подсветки нет и фронтенду знать ответ незачем.
func WithoutAnswers(qs []*Question) []Question {
	out := make([]Question, len(qs))
	for i, q := range qs {
		c := *q
		c.Answer = 0
		out[i] = c
	}
	return out
}
