package ui

type InputComponent interface {
	Component

	SetSize(width, height int)
	Value() string
	setValue(string)
	Focus()
	Blur()
	Focused() bool
}

type InputModel struct {
	value   string
	cursor  int
	focused bool
	width   int
	height  int
}

func CreateInput() InputComponent {

}
