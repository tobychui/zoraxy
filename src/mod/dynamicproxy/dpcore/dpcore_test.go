package dpcore_test

import (
	"testing"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
)

func TestReplaceLocationHost(t *testing.T) {
	urlString := "http://private.com/test/newtarget/"
	rrr := &dpcore.ResponseRewriteRuleSet{
		OriginalHost: "test.example.com",
		ProxyDomain:  "private.com/test",
		UseTLS:       true,
	}
	useTLS := true

	expectedResult := "https://test.example.com/newtarget/"

	result, err := dpcore.ReplaceLocationHost(urlString, rrr, useTLS)
	if err != nil {
		t.Errorf("Error occurred: %v", err)
	}

	if result != expectedResult {
		t.Errorf("Expected: %s, but got: %s", expectedResult, result)
	}
}

func TestReplaceLocationHostRelative(t *testing.T) {
	urlString := "api/"
	rrr := &dpcore.ResponseRewriteRuleSet{
		OriginalHost: "test.example.com",
		ProxyDomain:  "private.com/test",
		UseTLS:       true,
	}
	useTLS := true

	expectedResult := "https://test.example.com/api/"

	result, err := dpcore.ReplaceLocationHost(urlString, rrr, useTLS)
	if err != nil {
		t.Errorf("Error occurred: %v", err)
	}

	if result != expectedResult {
		t.Errorf("Expected: %s, but got: %s", expectedResult, result)
	}
}
