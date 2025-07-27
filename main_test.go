package main

import (
	"testing"
)

func TestCleanChirpBody(t *testing.T) {
	// Test Valid chirp
	input := "This is a valid chirp"
	expected := "This is a valid chirp"

	actual := cleanChirpBody(input)
	if actual != expected {
		t.Errorf("Test failed: Expected: %s Actual: %s", expected, actual)
	}

	// Test chirp with naughty words
	input = "This chirp has kerfuffle sharbert and fornax in it"
	expected = "This chirp has **** **** and **** in it"

	actual = cleanChirpBody(input)
	if actual != expected {
		t.Errorf("Test failed: Expected: %s Actual: %s", expected, actual)
	}

	// Test function does not clean naughty words attached to punctuation

	input = "kerfuffle!"
	expected = "kerfuffle!"

	actual = cleanChirpBody(input)
	if actual != expected {
		t.Errorf("Test failed: Expected: %s Actual: %s", expected, actual)
	}

}
