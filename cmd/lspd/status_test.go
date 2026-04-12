package main

import "testing"

func TestStringValueHandlesCommonTypes(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		input any
		want  string
	}{
		"string":  {input: "value", want: "value"},
		"int":     {input: 7, want: "7"},
		"float64": {input: 7.0, want: "7"},
		"bool":    {input: true, want: "true"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := stringValue(tc.input); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestIntValueHandlesNumericInputs(t *testing.T) {
	t.Parallel()

	if got, ok := intValue(42); !ok || got != 42 {
		t.Fatalf("expected int value 42, got %d ok=%v", got, ok)
	}
	if got, ok := intValue(42.0); !ok || got != 42 {
		t.Fatalf("expected float value 42, got %d ok=%v", got, ok)
	}
	if _, ok := intValue("nope"); ok {
		t.Fatal("expected string input to fail conversion")
	}
}
