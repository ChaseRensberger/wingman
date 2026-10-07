package store

// MessageQuery selects messages in absolute history order.
type MessageQuery struct {
	BoundaryPart string
	RunID        string
	MessageID    string
	Role         string
	State        string
}

// ModelCallQuery selects attempts by run or by their assistant's history index.
type ModelCallQuery struct {
	RunID            string
	FromMessageIndex int
}
