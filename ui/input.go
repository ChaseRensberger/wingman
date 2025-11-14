package ui

// import (
// 	"charm.land/bubbles/v2/textarea"
// )

type Input interface {
	Component
	Focusable
	Sizeable
}

type InputComponent struct {
	width, height int
	x, y          int
	// textarea      textarea.Model
}
