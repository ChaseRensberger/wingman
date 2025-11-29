package models

type WingmanMessage struct {
	Role    string
	Content string
}

type WingmanContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type WingmanUsage struct {
	InputTokens  int
	OutputTokens int
}

type WingmanMessageResponse struct {
	ID         string
	Content    []WingmanContentBlock
	StopReason string
	Usage      WingmanUsage
}
