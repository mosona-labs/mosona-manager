package pbTypes

type XTermMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}
