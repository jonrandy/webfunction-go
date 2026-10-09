package webfunction

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func parseType(t *testing.T, raw string) Type {
	t.Helper()
	var ty Type
	if err := json.Unmarshal([]byte(raw), &ty); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	return ty
}

// Every pre-existing form must parse exactly as before.
func TestTypeParseExistingFormsUnchanged(t *testing.T) {
	cases := map[string]string{
		`"string"`:                    "string",
		`"string.email"`:              "string.email",
		`["string","null"]`:           "string|null",
		`[["string"]]`:                "array<string>",
		`[["object.person","null"]]`:  "array<object.person|null>",
		`["array",["object.person"]]`: "array|array<object.person>",
		`null`:                        "any",
		`"object.user"`:               "object.user",
	}
	for raw, want := range cases {
		if got := parseType(t, raw).String(); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

func TestTypeApplicationSingle(t *testing.T) {
	ty := parseType(t, `{ "object.list": "object.user" }`)
	if got := ty.String(); got != "object.list<object.user>" {
		t.Fatalf("String: got %q", got)
	}
	if len(ty.Union) != 1 {
		t.Fatalf("want 1 alternative, got %d", len(ty.Union))
	}
	alt := ty.Union[0]
	if alt.Base != "object" || alt.Refinement != "list" || !alt.IsObjectRef() {
		t.Errorf("unexpected alt: %+v", alt)
	}
	if alt.Arg == nil || alt.Arg.String() != "object.user" {
		t.Errorf("unexpected Arg: %+v", alt.Arg)
	}
	if alt.Malformed != "" {
		t.Errorf("should not be malformed: %q", alt.Malformed)
	}
}

func TestTypeApplicationUnionArgument(t *testing.T) {
	ty := parseType(t, `{ "object.list": ["object.user", "null"] }`)
	if got := ty.String(); got != "object.list<object.user|null>" {
		t.Fatalf("got %q", got)
	}
	if len(ty.Union[0].Arg.Union) != 2 {
		t.Errorf("want a 2-member union argument, got %+v", ty.Union[0].Arg)
	}
}

func TestTypeApplicationNested(t *testing.T) {
	ty := parseType(t, `{ "object.page": { "object.page": "object.user" } }`)
	if got := ty.String(); got != "object.page<object.page<object.user>>" {
		t.Fatalf("got %q", got)
	}
	got := ty.ObjectNames()
	want := []string{"page", "page", "user"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ObjectNames: got %v, want %v", got, want)
	}
}

func TestTypeApplicationInsideUnionAndArrays(t *testing.T) {
	cases := map[string]string{
		`[{ "object.list": "object.user" }, "null"]`:               "object.list<object.user>|null",
		`[[{ "object.list": "object.user" }]]`:                     "array<object.list<object.user>>",
		`[[{ "object.list": "object.user" }, "null"]]`:             "array<object.list<object.user>|null>",
		`{ "object.list": [["string"]] }`:                          "object.list<array<string>>",
		`{ "object.list": [{ "object.pair": "number" }, "null"] }`: "object.list<object.pair<number>|null>",
	}
	for raw, want := range cases {
		if got := parseType(t, raw).String(); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// Malformed instantiation objects must never fail parsing: they resolve to
// "any", with the reason recorded so a validator can report it distinctly.
func TestTypeApplicationMalformed(t *testing.T) {
	cases := []struct {
		raw    string
		reason string
	}{
		{`{}`, MalformedKeyCount},
		{`{ "object.a": "string", "object.b": "string" }`, MalformedKeyCount},
		{`{ "string": "object.user" }`, MalformedKeyPrefix},
		{`{ "list": "object.user" }`, MalformedKeyPrefix},
		{`{ "object.": "string" }`, MalformedKeyPrefix},
		{`{ "object.list": 5 }`, MalformedArgument},
	}
	for _, c := range cases {
		ty := parseType(t, c.raw)
		if len(ty.Union) != 1 {
			t.Errorf("%s: want 1 alternative, got %d", c.raw, len(ty.Union))
			continue
		}
		alt := ty.Union[0]
		if alt.Base != "any" || alt.Refinement != "" || alt.Arg != nil {
			t.Errorf("%s: want bare any, got %+v", c.raw, alt)
		}
		if alt.Malformed != c.reason {
			t.Errorf("%s: Malformed = %q, want %q", c.raw, alt.Malformed, c.reason)
		}
		if !ty.Valid(map[string]any{"x": 1.0}) || !ty.Valid("anything") {
			t.Errorf("%s: a malformed application should accept any value", c.raw)
		}
	}

	// A malformed entry inside a union is kept as "any", not dropped.
	ty := parseType(t, `[{ "string": "x" }, "null"]`)
	if got := ty.String(); got != "any|null" {
		t.Errorf("malformed entry in a union: got %q, want %q", got, "any|null")
	}
	if ty.Union[0].Malformed != MalformedKeyPrefix {
		t.Errorf("reason lost inside a union: %+v", ty.Union[0])
	}
}

func TestTypeApplicationFormatAndValid(t *testing.T) {
	ty := parseType(t, `{ "object.list": "object.user" }`)
	if got := ty.Format("compact"); got != "list" {
		t.Errorf("compact: got %q", got)
	}
	if got := ty.Format("base"); got != "object" {
		t.Errorf("base: got %q", got)
	}
	if !ty.HasBase("object") {
		t.Error("HasBase(object) should be true")
	}
	if !ty.Valid(map[string]any{}) || ty.Valid("nope") {
		t.Error("an applied object type validates as an object")
	}
}

// "T" is just a bare token to the parser; resolving it is the job of a
// later layer. It must parse without error, including inside arrays/unions.
func TestTypeParamTokenParses(t *testing.T) {
	cases := map[string]string{
		`"T"`:          "T",
		`[["T"]]`:      "array<T>",
		`["T","null"]`: "T|null",
	}
	for raw, want := range cases {
		if got := parseType(t, raw).String(); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// The original failure mode: a package using generics made the whole
// package fail to load.
func TestPackageWithGenericsLoads(t *testing.T) {
	var baseURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{
			"base_url": baseURL,
			"endpoints": []any{
				map[string]any{
					"name":      "list-users",
					"returns":   map[string]any{"object.list": "object.user"},
					"arguments": []any{map[string]any{"name": "filter", "type": []any{map[string]any{"object.pair": "string"}, "null"}}},
				},
				map[string]any{"name": "broken", "returns": map[string]any{}, "arguments": []any{}},
			},
			"objects": []any{
				map[string]any{"name": "list", "attributes": []any{
					map[string]any{"name": "total", "type": "number"},
					map[string]any{"name": "items", "type": []any{[]any{"T"}}},
				}},
			},
		})
	}))
	defer srv.Close()
	baseURL = srv.URL + "/"

	c, err := FromPackageEndpoint(srv.URL+"/package", Options{})
	if err != nil {
		t.Fatalf("FromPackageEndpoint failed on a package using generics: %v", err)
	}
	pkg := c.Package()
	if got := pkg.Endpoint("list-users").Returns.String(); got != "object.list<object.user>" {
		t.Errorf("returns: got %q", got)
	}
	if got := pkg.Endpoint("list-users").Arguments[0].Type.String(); got != "object.pair<string>|null" {
		t.Errorf("argument type: got %q", got)
	}
	if got := pkg.Endpoint("broken").Returns.String(); got != "any" {
		t.Errorf("malformed returns: got %q, want any", got)
	}
	if got := pkg.Object("list").Attributes[1].Type.String(); got != "array<T>" {
		t.Errorf("T attribute: got %q", got)
	}
}