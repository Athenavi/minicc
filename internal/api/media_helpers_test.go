package api

import (
	"errors"
	"net/url"
	"testing"
)

func TestWouldCreateCycle(t *testing.T) {
	parentOf := map[string]string{
		"a": "", "b": "a", "c": "b", // a 鈫?root; b 鈫?a; c 鈫?b
	}
	getParent := func(id string) (string, error) {
		p, ok := parentOf[id]
		if !ok {
			return "", errors.New("not found")
		}
		return p, nil
	}

	cases := []struct {
		name      string
		id, newP  string
		wantCycle bool
	}{
		{"move c under a is fine", "c", "a", false},
		{"move a under itself", "a", "a", true},
		{"move b under c creates cycle", "b", "c", true},
		{"move a under c creates cycle", "a", "c", true},
		{"move c to root is fine", "c", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cycle, err := wouldCreateCycle(getParent, c.id, c.newP)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cycle != c.wantCycle {
				t.Errorf("wouldCreateCycle(%q -> %q) = %v, want %v", c.id, c.newP, cycle, c.wantCycle)
			}
		})
	}
}

func TestCollectFolderIDs(t *testing.T) {
	children := map[string][]string{
		"root": {"f1", "f2"},
		"f1":   {"d1", "d2"},
		"d1":   {"x"},
	}
	getChildren := func(parent string) ([]string, error) { return children[parent], nil }

	ids, err := collectFolderIDs(getChildren, "root")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id] = true
	}
	for _, want := range []string{"root", "f1", "f2", "d1", "d2", "x"} {
		if !got[want] {
			t.Errorf("collectFolderIDs missing %q", want)
		}
	}
	if len(ids) != 6 {
		t.Errorf("expected 6 ids, got %d: %v", len(ids), ids)
	}
}

func TestParsePagination(t *testing.T) {
	q := func(p, s string) url.Values {
		v := url.Values{}
		if p != "" {
			v.Set("page", p)
		}
		if s != "" {
			v.Set("page_size", s)
		}
		return v
	}
	cases := []struct {
		p, s     string
		wantPage int
		wantSize int
	}{
		{"", "", 1, 50},
		{"3", "", 3, 50},
		{"0", "-5", 1, 50},
		{"2", "100", 2, 100},
		{"1", "999", 1, 200}, // cap at 200
	}
	for _, c := range cases {
		page, size := parsePagination(q(c.p, c.s))
		if page != c.wantPage || size != c.wantSize {
			t.Errorf("parsePagination(page=%q,size=%q) = (%d,%d), want (%d,%d)", c.p, c.s, page, size, c.wantPage, c.wantSize)
		}
	}
}
