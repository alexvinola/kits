package rootfile

import (
	"strings"
	"testing"
)

func TestUpsertAppendReplaceRemove(t *testing.T) {
	doc := []byte("# Project\n\nMine.")
	doc, err := Upsert(doc, "core", []byte("v1"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# Project\n\nMine.\n\n<!-- kits:begin core -->\nv1\n<!-- kits:end core -->\n"
	if string(doc) != want {
		t.Fatalf("append:\n%q\nwant\n%q", doc, want)
	}

	doc = append(doc, "\nAfter.\n"...)
	doc, err = Upsert(doc, "core", []byte("v2\n"))
	if err != nil {
		t.Fatal(err)
	}
	want = "# Project\n\nMine.\n\n<!-- kits:begin core -->\nv2\n<!-- kits:end core -->\n\nAfter.\n"
	if string(doc) != want {
		t.Fatalf("replace:\n%q\nwant\n%q", doc, want)
	}

	again, _ := Upsert(doc, "core", []byte("v2\n"))
	if string(again) != string(doc) {
		t.Fatal("upsert is not idempotent")
	}

	doc, err = Remove(doc, "core")
	if err != nil {
		t.Fatal(err)
	}
	if want := "# Project\n\nMine.\n\n\nAfter.\n"; string(doc) != want {
		t.Fatalf("remove middle:\n%q\nwant\n%q", doc, want)
	}
}

func TestRemoveLastBlock(t *testing.T) {
	doc, _ := Upsert([]byte("Mine.\n"), "core", []byte("x"))
	doc, err := Remove(doc, "core")
	if err != nil || string(doc) != "Mine.\n" {
		t.Fatalf("got %q, %v", doc, err)
	}
}

func TestUpsertEmptyDoc(t *testing.T) {
	doc, _ := Upsert(nil, "core", []byte("x\n"))
	if want := "<!-- kits:begin core -->\nx\n<!-- kits:end core -->\n"; string(doc) != want {
		t.Fatalf("got %q", doc)
	}
}

func TestMalformed(t *testing.T) {
	cases := map[string]string{
		"no end":    "<!-- kits:begin core -->\nx\n",
		"no begin":  "x\n<!-- kits:end core -->\n",
		"two begin": "<!-- kits:begin core -->\n<!-- kits:begin core -->\n<!-- kits:end core -->\n",
		"two end":   "<!-- kits:begin core -->\n<!-- kits:end core -->\n<!-- kits:end core -->\n",
	}
	for name, doc := range cases {
		if _, err := Upsert([]byte(doc), "core", []byte("y")); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestBodyWithMarker(t *testing.T) {
	_, err := Upsert(nil, "core", []byte("a\n<!-- kits:end ia -->\n"))
	if err == nil || !strings.Contains(err.Error(), "marker") {
		t.Fatalf("err = %v", err)
	}
}
