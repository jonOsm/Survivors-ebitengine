package main

import "math"

// calculateXPForLevel determines the total XP needed to advance from the currentLevel to currentLevel + 1.
// For example, if currentLevel is 0, this is the XP needed to reach Level 1.
// If currentLevel is 1, this is the XP needed to reach Level 2.
func calculateXPForLevel(currentLevel int) int {
	if currentLevel < 0 {
		currentLevel = 0 // Should not happen, but safety first
	}
	baseXP := 100.0 // Use float for Pow calculation precision

	// For level 0, the cost is just baseXP.
	// For subsequent levels, add a scaled power of the level.
	if currentLevel == 0 {
		return int(baseXP)
	}
	// XP required = Base + (Level ^ Exponent) * Factor
	// Example: 100 + (Lvl^1.7) * 75
	additionalXP := math.Pow(float64(currentLevel), 1.7) * (baseXP * 0.75)
	return int(baseXP + additionalXP)
}
