package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGreenContainsText(t *testing.T) {
	result := Green("hello")
	assert.Contains(t, result, "hello")
}

func TestYellowContainsText(t *testing.T) {
	result := Yellow("warning")
	assert.Contains(t, result, "warning")
}

func TestRedContainsText(t *testing.T) {
	result := Red("error")
	assert.Contains(t, result, "error")
}

func TestCyanContainsText(t *testing.T) {
	result := Cyan("info")
	assert.Contains(t, result, "info")
}

func TestColorFunctionsEmptyStringDoNotPanic(t *testing.T) {
	assert.NotPanics(t, func() { Green("") })
	assert.NotPanics(t, func() { Yellow("") })
	assert.NotPanics(t, func() { Red("") })
	assert.NotPanics(t, func() { Cyan("") })
}
