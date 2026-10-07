package desktop

// Accessibility is the macOS display accessibility settings the UI honors.
type Accessibility struct {
	ReduceTransparency bool `json:"reduceTransparency"`
	ReduceMotion       bool `json:"reduceMotion"`
	IncreaseContrast   bool `json:"increaseContrast"`
}
