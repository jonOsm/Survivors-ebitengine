package main

// calculateXPForLevel determines the total XP needed to advance from the currentLevel to currentLevel + 1.
// For example, if currentLevel is 0, this is the XP needed to reach Level 1.
// If currentLevel is 1, this is the XP needed to reach Level 2.
func calculateXPForLevel(currentLevel int) int {
	if currentLevel < 0 { // Should not happen, but good practice
		currentLevel = 0
	}
	return 100 + currentLevel*50
}
