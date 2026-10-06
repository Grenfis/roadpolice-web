package bank

import (
	"math/rand"
	"os"
	"testing"
)

func load(t *testing.T) *Bank {
	t.Helper()
	data, err := os.ReadFile("../../../bank/bank.json")
	if os.IsNotExist(err) {
		t.Skip("банк не собран: скопируйте bank/ из roadpolice-trainer")
	}
	if err != nil {
		t.Fatalf("читаю банк: %v", err)
	}
	b, err := Load(data)
	if err != nil {
		t.Fatalf("разбор банка: %v", err)
	}
	return b
}

func TestBankContents(t *testing.T) {
	b := load(t)
	if got := len(b.Questions); got != 1050 {
		t.Errorf("вопросов %d, ожидалось 1050", got)
	}
	want := map[string]int{"law": 82, "pdd": 858, "faults": 59, "first_aid": 51}
	for sec, n := range want {
		if got := b.CountBySection()[sec]; got != n {
			t.Errorf("раздел %s: %d вопросов, ожидалось %d", sec, got, n)
		}
	}
	if got := b.SectionIDs(); len(got) != 4 || got[0] != "law" || got[1] != "pdd" {
		t.Errorf("порядок разделов неверный: %v", got)
	}
	for _, q := range b.Questions {
		if len(q.Options) < 2 || len(q.Options) > 5 {
			t.Fatalf("%s: вариантов %d (должно быть 2..5)", q.ID, len(q.Options))
		}
	}
}

func TestTicketQuotas(t *testing.T) {
	b := load(t)
	rnd := rand.New(rand.NewSource(1))
	for i := 0; i < 200; i++ {
		qs, err := b.Ticket(rnd)
		if err != nil {
			t.Fatalf("билет: %v", err)
		}
		if len(qs) != 20 {
			t.Fatalf("в билете %d вопросов, ожидалось 20", len(qs))
		}
		bySec := map[string]int{}
		seen := map[string]bool{}
		for _, q := range qs {
			bySec[q.Section]++
			if seen[q.ID] {
				t.Fatalf("вопрос %s в билете дважды", q.ID)
			}
			seen[q.ID] = true
			if q.Answer < 1 {
				t.Fatalf("%s: нет правильного ответа", q.ID)
			}
		}
		want := map[string]int{"law": 2, "pdd": 15, "faults": 2, "first_aid": 1}
		for sec, n := range want {
			if bySec[sec] != n {
				t.Fatalf("раздел %s: %d вопросов в билете, по квоте %d", sec, bySec[sec], n)
			}
		}
	}
}

func TestTicketIsRandom(t *testing.T) {
	b := load(t)
	rnd := rand.New(rand.NewSource(42))
	first, _ := b.Ticket(rnd)
	second, _ := b.Ticket(rnd)
	same := 0
	for i := range first {
		if first[i].ID == second[i].ID {
			same++
		}
	}
	if same == len(first) {
		t.Error("два билета подряд совпали целиком — выбор не случайный")
	}
}

func TestSelect(t *testing.T) {
	b := load(t)
	rnd := rand.New(rand.NewSource(7))

	all, err := b.Select(Selection{Scope: "all"}, rnd)
	if err != nil || len(all) != 1050 {
		t.Fatalf("вся категория: %d вопросов, err=%v", len(all), err)
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].Group > all[i].Group {
			t.Fatal("порядок без перемешивания должен идти по группам")
		}
		// внутри группы — по номеру вопроса, а не по строке id
		// (иначе было бы q1, q10, q100, q2…)
		if all[i-1].Group == all[i].Group && all[i-1].Number+1 != all[i].Number {
			t.Fatalf("после вопроса %d идёт %d (группа %d)",
				all[i-1].Number, all[i].Number, all[i].Group)
		}
	}
	if all[0].Number != 1 {
		t.Errorf("первый вопрос имеет номер %d", all[0].Number)
	}

	g3, err := b.Select(Selection{Scope: "group", Group: 3}, rnd)
	if err != nil || len(g3) != 59 {
		t.Fatalf("группа 3: %d вопросов, err=%v", len(g3), err)
	}

	law, err := b.Select(Selection{Scope: "section", Section: "law"}, rnd)
	if err != nil || len(law) != 82 {
		t.Fatalf("раздел law: %d вопросов, err=%v", len(law), err)
	}

	ids, err := b.Select(Selection{Scope: "ids", IDs: []string{g3[5].ID, "нет-такого", law[0].ID}}, rnd)
	if err != nil {
		t.Fatalf("выборка по id: %v", err)
	}
	if len(ids) != 2 || ids[0].ID != g3[5].ID || ids[1].ID != law[0].ID {
		t.Errorf("выборка по id вернула %v", ids)
	}

	if _, err := b.Select(Selection{Scope: "group", Group: 77}, rnd); err == nil {
		t.Error("несуществующая группа должна давать ошибку")
	}

	shuffled, _ := b.Select(Selection{Scope: "all", Shuffle: true}, rnd)
	diff := 0
	for i := range all {
		if all[i].ID != shuffled[i].ID {
			diff++
		}
	}
	if diff < len(all)/2 {
		t.Errorf("перемешивание почти не изменило порядок (%d из %d)", diff, len(all))
	}
}

func TestWithoutAnswers(t *testing.T) {
	b := load(t)
	qs, _ := b.Ticket(rand.New(rand.NewSource(3)))
	hidden := WithoutAnswers(qs)
	for i, q := range hidden {
		if q.Answer != 0 {
			t.Fatalf("%s: ответ не скрыт", q.ID)
		}
		if qs[i].Answer == 0 {
			t.Fatalf("%s: исходный вопрос испорчен", q.ID)
		}
	}
}

func TestLoadRejectsBadBank(t *testing.T) {
	cases := map[string]string{
		"пустой банк":     `{"meta":{},"questions":[]}`,
		"ответ вне опций": `{"meta":{},"questions":[{"id":"a","options":["x","y"],"answer":3}]}`,
		"дубль id":        `{"meta":{},"questions":[{"id":"a","options":["x"],"answer":1},{"id":"a","options":["x"],"answer":1}]}`,
	}
	for name, data := range cases {
		if _, err := Load([]byte(data)); err == nil {
			t.Errorf("%s: ожидалась ошибка", name)
		}
	}
}
