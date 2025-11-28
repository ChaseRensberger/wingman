package agent

type Agent struct {
	name string
}

func CreateAgent(name string) *Agent {
	return &Agent{
		name: name,
	}
}
