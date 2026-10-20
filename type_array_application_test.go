package webfunction

import (
	"encoding/json"
	"reflect"
	"testing"
)

// { "array": X } is the canonical way to write an array of X (spec: Generics,
// Application). It must parse to exactly the same model as the legacy
// [[X]] form, so every consumer sees one representation.
func TestTypeArrayApplicationEquivalentToLegacyForm(t *testing.T) {
	cases := []struct{ application, legacy string }{
		{`{ "array": "string" }`, `[["string"]]`},
		{`{ "array": "object.user" }`, `[["object.user"]]`},
		{`{ "array": ["string", "null"] }`, `[["string", "null"]]`},
		{`{ "array": "T" }`, `[["T"]]`},
		{`{ "array": { "array": "string" } }`, `[[["string"]]]`},
		{`{ "array": { "object.page": "object.user" } }`, `[[{ "object.page": "object.user" }]]`},
		{`{ "object.page": { "array": "object.user" } }`, `{ "object.page": [["object.user"]] }`},
		{`[{ "array": "string" }, "null"]`, `[["string"], "null"]`},
	}
	for _, c := range cases {
		got, want := parseType(t, c.application), parseType(t, c.legacy)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: parsed as %+v, want the same as %s (%+v)", c.application, got, c.legacy, want)
		}
	}
}

func TestTypeArrayApplicationString(t *testing.T) {
	cases := map[string]string{
		`{ "array": "string" }`:                         "array<string>",
		`{ "array": "object.user" }`:                    "array<object.user>",
		`{ "array": ["string", "number"] }`:             "array<string|number>",
		`{ "array": { "array": "string" } }`:            "array<array<string>>",
		`{ "array": { "object.page": "object.user" } }`: "array<object.page<object.user>>",
		`{ "object.page": { "array": "object.user" } }`: "object.page<array<object.user>>",
		`{ "array": "T" }`:                              "array<any>",
		`[{ "array": "string" }, "null"]`:                "array<string>|null",
		`["array", { "array": "string" }]`:              "array|array<string>",
	}
	for raw, want := range cases {
		if got := parseType(t, raw).String(); got != want {
			t.Errorf("%s: got %q, want %q", raw, got, want)
		}
	}
}

// A union of one entry is that entry (spec: Substitution).
func TestTypeArrayApplicationInSingleEntryUnion(t *testing.T) {
	ty := parseType(t, `[{ "array": "string" }]`)
	if len(ty.Union) != 1 || ty.Union[0].Base != "array" || ty.Union[0].Of == nil {
		t.Errorf("a one-entry union holding an array application should be that array: %+v", ty)
	}
}

func TestTypeArrayApplicationShape(t *testing.T) {
	ty := parseType(t, `{ "array": "object.user" }`)
	if len(ty.Union) != 1 {
		t.Fatalf("want 1 alternative, got %d", len(ty.Union))
	}
	alt := ty.Union[0]
	if alt.Base != "array" || alt.Refinement != "" {
		t.Errorf("unexpected alt: %+v", alt)
	}
	if alt.Of == nil || alt.Of.String() != "object.user" {
		t.Errorf("unexpected Of: %+v", alt.Of)
	}
	if alt.Arg != nil || alt.Malformed != "" {
		t.Errorf("an array application is an ordinary array, not an object application: %+v", alt)
	}
	if !ty.HasBase("array") || ty.HasBareArray() {
		t.Errorf("HasBase(array) should be true and HasBareArray false for %s", ty)
	}

	param := parseType(t, `{ "array": "T" }`).Union[0]
	if param.Of == nil || !param.Of.Union[0].Param {
		t.Errorf("T must survive as the element of an array application: %+v", param)
	}
}

func TestTypeArrayApplicationObjectNames(t *testing.T) {
	cases := map[string][]string{
		`{ "array": "object.user" }`:                    {"user"},
		`{ "array": { "object.page": "object.user" } }`: {"page", "user"},
		`{ "object.page": { "array": "object.user" } }`: {"page", "user"},
		`{ "array": { "array": "object.user" } }`:       {"user"},
	}
	for raw, want := range cases {
		if got := parseType(t, raw).ObjectNames(); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: ObjectNames got %v, want %v", raw, got, want)
		}
	}
}

func TestTypeArrayApplicationValid(t *testing.T) {
	strings := parseType(t, `{ "array": "string" }`)
	if !strings.Valid([]any{"a", "b"}) || !strings.Valid([]any{}) {
		t.Error("an array of strings should accept string items")
	}
	if strings.Valid([]any{1.0}) || strings.Valid("a") || strings.Valid(nil) {
		t.Error("an array of strings must reject non-string items and non-arrays")
	}

	mixed := parseType(t, `{ "array": ["string", "number"] }`)
	if !mixed.Valid([]any{"a", 1.0}) || mixed.Valid([]any{true}) {
		t.Error("the element union applies to every item")
	}

	nested := parseType(t, `{ "array": { "array": "number" } }`)
	if !nested.Valid([]any{[]any{1.0}, []any{}}) || nested.Valid([]any{[]any{"a"}}) {
		t.Error("nested array applications validate each level")
	}
}

// Everything that was malformed before is still malformed, with the same
// reasons; an array application only changes what a key of exactly "array"
// means.
func TestTypeArrayApplicationMalformed(t *testing.T) {
	cases := []struct{ raw, reason string }{
		{`{ "array": "string", "object.a": "string" }`, MalformedKeyCount},
		{`{ "array": 5 }`, MalformedArgument},
		{`{ "arrays": "string" }`, MalformedKeyPrefix},
		{`{ "array.x": "string" }`, MalformedKeyPrefix},
		{`{ "Array": "string" }`, MalformedKeyPrefix},
		{`{ "string": "object.user" }`, MalformedKeyPrefix},
		{`{}`, MalformedKeyCount},
	}
	for _, c := range cases {
		ty := parseType(t, c.raw)
		if len(ty.Union) != 1 {
			t.Fatalf("%s: want 1 alternative, got %d", c.raw, len(ty.Union))
		}
		alt := ty.Union[0]
		if alt.Base != "any" || alt.Malformed != c.reason {
			t.Errorf("%s: got Base %q Malformed %q, want any / %q", c.raw, alt.Base, alt.Malformed, c.reason)
		}
		if !ty.Valid("anything") {
			t.Errorf("%s: a malformed application should accept any value", c.raw)
		}
	}
}

// A bare "array" is still an array of any.
func TestTypeBareArrayUnchanged(t *testing.T) {
	ty := parseType(t, `"array"`)
	if !ty.HasBareArray() || ty.Union[0].Of != nil {
		t.Errorf("bare array should stay array of any: %+v", ty)
	}
}

// The spec's own recursion example, plus an array application in a return
// type, must load into the ordinary model.
func TestPackageWithArrayApplicationLoads(t *testing.T) {
	var pkg Package
	err := json.Unmarshal([]byte(`{
	  "base_url": "https://api.example.com",
	  "name": "ExamplePackage",
	  "objects": [
	    {
	      "name": "comment",
	      "attributes": [
	        { "name": "id", "type": "string.uuid" },
	        { "name": "message", "type": "string" },
	        { "name": "replies", "type": { "array": "object.comment" } }
	      ]
	    }
	  ],
	  "endpoints": [
	    { "name": "get-comments", "returns": { "array": "object.comment" }, "arguments": [] }
	  ]
	}`), &pkg)
	if err != nil {
		t.Fatalf("a package using array applications must load: %v", err)
	}
	if got := pkg.Endpoint("get-comments").Returns.String(); got != "array<object.comment>" {
		t.Errorf("returns: got %q", got)
	}
	if got := pkg.Object("comment").Attributes[2].Type.String(); got != "array<object.comment>" {
		t.Errorf("replies: got %q", got)
	}
}