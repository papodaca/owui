package tui

import "testing"

func TestFuzzyRanksWordStarts(t *testing.T) {
	items := []pickerItem{
		{id: "1", label: "summer reading"},
		{id: "2", label: "Maine race outlook"},
		{id: "3", label: "unrelated"},
	}
	got := fuzzyItems(items, "mr")
	if len(got) != 2 {
		t.Fatalf("len = %d %#v", len(got), got)
	}
	if got[0].id != "2" {
		t.Fatalf("first = %#v", got[0])
	}
}

func TestFuzzyRequiresOrder(t *testing.T) {
	items := []pickerItem{{id: "1", label: "race"}}
	if got := fuzzyItems(items, "ar"); len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
}
