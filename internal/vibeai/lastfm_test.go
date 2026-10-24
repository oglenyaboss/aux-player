package vibeai

import (
	"reflect"
	"strings"
	"testing"
)

func TestFilterTags(t *testing.T) {
	tags := []tag{
		{Name: "2024", Count: 100},
		{Name: "trap", Count: 100},
		{Name: "Hip-Hop", Count: 16},
		{Name: "new jazz", Count: 16},
		{Name: "pop rap", Count: 8},
		{Name: "rage", Count: 8},
		{Name: "Southern Hip Hop", Count: 8},
		{Name: "alternative rnb", Count: 3},
		{Name: "7", Count: 3},
		{Name: "2020s", Count: 3},
		{Name: "hip hop", Count: 2},
		{Name: "don toliver", Count: 1},
	}
	tests := []struct {
		name string
		n    int
		want []string
	}{
		{"n=5", 5, []string{"trap", "Hip-Hop", "new jazz", "pop rap", "rage"}},
		{"n=10", 10, []string{"trap", "Hip-Hop", "new jazz", "pop rap", "rage", "Southern Hip Hop", "alternative rnb"}},
	}
	for _, tt := range tests {
		got := filterTags(tags, "Don Toliver", tt.n)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestParseTopTags(t *testing.T) {
	got, err := parseTopTags([]byte(`{"toptags":{"tag":[{"name":"trap","count":100},{"name":"rage","count":"8"}]}}`))
	if err != nil {
		t.Fatalf("array: unexpected err %v", err)
	}
	want := []tag{{Name: "trap", Count: 100}, {Name: "rage", Count: 8}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("array: got %v, want %v", got, want)
	}
	got, err = parseTopTags([]byte(`{"toptags":{"tag":{"name":"solo","count":5}}}`))
	if err != nil {
		t.Fatalf("single: unexpected err %v", err)
	}
	want = []tag{{Name: "solo", Count: 5}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("single: got %v, want %v", got, want)
	}
	_, err = parseTopTags([]byte(`{"error":6,"message":"Album not found"}`))
	if err == nil || !strings.Contains(err.Error(), "6") || !strings.Contains(err.Error(), "Album not found") {
		t.Errorf("error envelope: err = %v, want text with 6 and \"Album not found\"", err)
	}
}
