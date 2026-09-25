package gui

import (
	"errors"
	"testing"
)

func TestDecodeGeneratedValuesChecked(t *testing.T) {
	values, err := decodeGeneratedValuesChecked(`{"name":"Vela","age":31}`)
	if err != nil {
		t.Fatalf("decodeGeneratedValuesChecked returned an error: %v", err)
	}
	if values["name"] != "Vela" || values["age"] != "31" {
		t.Fatalf("values = %v, want name and age", values)
	}

	if _, err := decodeGeneratedValuesChecked("I am afraid I cannot do that."); !errors.Is(err, ErrNoDecodableFields) {
		t.Fatalf("err = %v, want ErrNoDecodableFields", err)
	}
}
