package match

// Helper function to compare float values with a small epsilon
func floatEquals(a, b float64) bool {
	const epsilon = 0.000001
	return (a-b) < epsilon && (b-a) < epsilon
}
