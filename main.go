package main

import (
	"image" // Added for image.Rect
	"image/color"
	"log"
	"math"
	"math/rand"
	"os" // For os.Exit
	"strconv"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil" // For IsKeyJustPressed
)

const (
	screenWidth     = 800
	screenHeight    = 600
	worldWidth      = screenWidth * 4
	worldHeight     = screenHeight * 4
	cameraLerpSpeed = 0.05 // Lower for smoother/slower lerp, higher for faster
)

var (
	playerImage      *ebiten.Image
	enemyImage       *ebiten.Image
	pulseAttackImage *ebiten.Image // Semi-transparent white for the pulse itself
	opaqueClearImage *ebiten.Image // Opaque white, used for CompositeModeClear
	obstacleImage    *ebiten.Image // Image for obstacles
	xpOrbImage       *ebiten.Image // Image for XP orbs
)

// GameState defines the current state of the game.
type GameState int

const (
	StateTitleScreen GameState = iota
	StateGameplay
	StateLoseScreen
)

// Game implements ebiten.Game interface.
type Game struct {
	player              *Player
	enemies             []*Enemy
	pulseAttacks        []*PulseAttack
	attackTimer         float64 // Seconds
	spawnTimer          float64 // Seconds
	score               int
	enemySpawnRate      float64 // Seconds between enemy spawns
	enemiesPerWave      int
	camX, camY          float64
	obstacles           []*Obstacle
	xpOrbs              []*XPOrb // Slice to hold active XP orbs
	levelUpMessageTimer float64  // For displaying "LEVEL UP!" message

	currentState        GameState
	titleSelectedOption int // 0 for Start, 1 for Quit
	loseSelectedOption  int // 0 for Retry, 1 for Main Menu
}

// XPOrb represents an experience point orb dropped by enemies.
type XPOrb struct {
	X, Y            float64
	Value           int // How much XP this orb gives
	Image           *ebiten.Image
	CollisionRadius float64
	Collected       bool // Flag for cleanup
}

// Obstacle represents a static object in the game world.
type Obstacle struct {
	X, Y    float64
	Width, Height float64
	Image   *ebiten.Image
}

// Player represents the player character.
type Player struct {
	X, Y  float64
	Speed float64
	Image *ebiten.Image
	CollisionRadius float64
	Level           int
	CurrentXP       int
	XPToNextLevel   int
	CurrentHealth   int
	MaxHealth       int
	invulnerabilityTimer float64 // Seconds
}

// Enemy represents an enemy character.
type Enemy struct {
	X, Y  float64
	Speed float64
	Health int
	Image *ebiten.Image
	CollisionRadius float64
}

const (
	playerCollisionRadius = 8 // Half of player image size
	enemyCollisionRadius  = 6 // Half of enemy image size
	enemySpeed            = 70 // Pixels per second (reduced from 100)
	enemyHealth           = 1   // Initial health, can be increased for difficulty
	defaultEnemySpawnRate = 3.0 // Seconds
	defaultEnemiesPerWave = 5
)

// PulseAttack represents an expanding attack.
type PulseAttack struct {
	X, Y           float64
	Radius         float64
	MaxRadius      float64
	ExpansionSpeed float64
	Image          *ebiten.Image // This will be dynamically resized or redrawn
}

const (
	pulseAttackCooldown       = 1.0 // seconds
	pulseAttackMaxRadius      = 32  // Player diameter 16px. 4x is 64px diameter. MaxRadius is half of that.
	pulseAttackExpansionSpeed = 40  // pixels per second.
	pulseRingThickness        = 4   // Thickness of the pulse ring in pixels.
	xpOrbValue                = 25  // XP gained per orb
	xpOrbCollisionRadius      = 15   // For collecting XP orbs (increased from 5; image is 10x10)
	levelUpMessageDuration    = 2.0 // seconds
	playerMaxHealth           = 100
	enemyContactDamage        = 10
	playerHitInvulnerabilityDuration = 0.75 // seconds
)

func init() {
	// Initialize images
	playerImage = ebiten.NewImage(16, 16)
	playerImage.Fill(color.White)

	enemyImage = ebiten.NewImage(12, 12)
	enemyImage.Fill(color.RGBA{R: 255, A: 255}) // Red

	// Pulse attack image is tricky. For now, a base image.
	// We'll handle its dynamic drawing/scaling later.
	// A fully transparent image that gets its alpha/color from drawing options might be better.
	// For simplicity, let's start with a semi-transparent white square.
	// We will draw this scaled up.
	pulseAttackImage = ebiten.NewImage(1, 1) // 1x1 pixel, will be scaled
	pulseAttackImage.Fill(color.RGBA{R: 255, G: 255, B: 255, A: 100}) // Semi-transparent white

	opaqueClearImage = ebiten.NewImage(1, 1)
	opaqueClearImage.Fill(color.White) // Fully opaque white

	obstacleImage = ebiten.NewImage(50, 50) // Example size
	obstacleImage.Fill(color.Gray{Y: 100})    // Dark gray

	xpOrbImage = ebiten.NewImage(10, 10)
	xpOrbImage.Fill(color.RGBA{B: 255, A: 255}) // Blue

	rand.Seed(time.Now().UnixNano())
}

// NewGame initializes a new game.
func NewGame() *Game {
	g := &Game{
		player: &Player{
			X:               worldWidth / 2,  // Start player at world center
			Y:               worldHeight / 2, // Start player at world center
			Speed:           200, // Pixels per second
			Image:           playerImage,
			CollisionRadius: playerCollisionRadius,
			Level:           0,
			CurrentXP:       0,
			XPToNextLevel:   calculateXPForLevel(0),
			MaxHealth:       playerMaxHealth,
			CurrentHealth:   playerMaxHealth,
			invulnerabilityTimer: 0,
		},
		enemies:        []*Enemy{},
		pulseAttacks:   []*PulseAttack{},
		attackTimer:    0,
		spawnTimer:     0,
		score:          0,
		// gameOver:       false, // Removed
		enemySpawnRate: defaultEnemySpawnRate,
		enemiesPerWave: defaultEnemiesPerWave,
		obstacles:      []*Obstacle{},
		xpOrbs:         []*XPOrb{},
		levelUpMessageTimer: 0,
		currentState:        StateTitleScreen,
		titleSelectedOption: 0, // Default to "Start Game"
		loseSelectedOption:  0, // Default to "Retry"
	}
	// Initialize camera to center on player
	g.camX = g.player.X - screenWidth/2
	g.camY = g.player.Y - screenHeight/2
	g.camX = clamp(g.camX, 0, worldWidth-screenWidth)    // Clamp initial camera position
	g.camY = clamp(g.camY, 0, worldHeight-screenHeight) // Clamp initial camera position

	g.initObstacles()
	return g
}

func (g *Game) initObstacles() {
	// For now, a few hardcoded obstacles.
	// Ensure positions are within worldWidth/worldHeight.
	// Obstacle X, Y is top-left corner.
	g.obstacles = []*Obstacle{
		{X: worldWidth/2 - 200, Y: worldHeight/2 - 25, Width: 100, Height: 50, Image: obstacleImage},
		{X: worldWidth/2 + 100, Y: worldHeight/2 - 25, Width: 100, Height: 50, Image: obstacleImage},
		{X: worldWidth/2 - 25, Y: worldHeight/2 - 200, Width: 50, Height: 100, Image: obstacleImage},
		{X: worldWidth/2 - 25, Y: worldHeight/2 + 100, Width: 50, Height: 100, Image: obstacleImage},
	}
}

// reset re-initializes the game state to its starting conditions.
func (g *Game) reset() {
	g.player.X = worldWidth / 2 // Player resets to world center
	g.player.Y = worldHeight / 2
	// Player speed and collision radius remain the same.

	// Reset level and XP
	g.player.Level = 0
	g.player.CurrentXP = 0
	g.player.XPToNextLevel = calculateXPForLevel(0)
	g.player.MaxHealth = playerMaxHealth
	g.player.CurrentHealth = playerMaxHealth
	g.player.invulnerabilityTimer = 0

	// Center camera on player
	g.camX = g.player.X - screenWidth/2
	g.camY = g.player.Y - screenHeight/2
	g.camX = clamp(g.camX, 0, worldWidth-screenWidth)
	g.camY = clamp(g.camY, 0, worldHeight-screenHeight)


	g.enemies = []*Enemy{}      // Clear enemies
	g.pulseAttacks = []*PulseAttack{} // Clear attacks
	g.xpOrbs = []*XPOrb{}       // Clear XP orbs

	g.attackTimer = 0
	g.spawnTimer = 0 // Reset spawn timer to allow immediate first wave on reset
	g.score = 0
	// g.gameOver = false // No longer needed here, state transitions handle flow
	g.levelUpMessageTimer = 0 // Reset level up message timer

	// Reset wave progression if it was dynamic (using defaults here)
	g.enemySpawnRate = defaultEnemySpawnRate
	g.enemiesPerWave = defaultEnemiesPerWave
}

// Update proceeds the game state.
// Update is called every tick (1/60 [s] by default).
func (g *Game) Update() error {
	switch g.currentState {
	case StateTitleScreen:
		g.updateTitleScreen()
	case StateGameplay:
		g.updateGameplayScreen()
	case StateLoseScreen:
		g.updateLoseScreen()
	}
	return nil
}

func (g *Game) updateTitleScreen() {
	if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
		g.titleSelectedOption--
		if g.titleSelectedOption < 0 {
			g.titleSelectedOption = 1 // Wrap around (0: Start, 1: Quit)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
		g.titleSelectedOption++
		if g.titleSelectedOption > 1 {
			g.titleSelectedOption = 0 // Wrap around
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		switch g.titleSelectedOption {
		case 0: // Start Game
			g.reset() // Reset game state for a fresh start
			g.currentState = StateGameplay
		case 1: // Quit
			os.Exit(0)
		}
	}
}

func (g *Game) updateGameplayScreen() {
	// NOTE: The g.gameOver check and immediate reset logic is removed from here.
	// Player death (CurrentHealth <= 0) will now trigger a state change to StateLoseScreen.

	// Player movement
	var dx, dy float64
	if ebiten.IsKeyPressed(ebiten.KeyW) || ebiten.IsKeyPressed(ebiten.KeyUp) {
		dy--
	}
	if ebiten.IsKeyPressed(ebiten.KeyS) || ebiten.IsKeyPressed(ebiten.KeyDown) {
		dy++
	}
	if ebiten.IsKeyPressed(ebiten.KeyA) || ebiten.IsKeyPressed(ebiten.KeyLeft) {
		dx--
	}
	if ebiten.IsKeyPressed(ebiten.KeyD) || ebiten.IsKeyPressed(ebiten.KeyRight) {
		dx++
	}

	// Normalize diagonal movement to maintain consistent speed.
	// If moving in two directions (e.g., W and D), the combined vector
	// would be longer than for cardinal movement. Dividing by sqrt(2)
	// ensures the magnitude of the movement vector is consistent.
	if dx != 0 && dy != 0 {
		dx /= math.Sqrt(2)
		dy /= math.Sqrt(2)
	}

	originalPlayerX := g.player.X // Position at start of this frame's Update()
	originalPlayerY := g.player.Y

	// Calculate proposed total movement for this frame
	dx_frame := dx * g.player.Speed / float64(ebiten.TPS())
	dy_frame := dy * g.player.Speed / float64(ebiten.TPS())

	// Target positions if no collisions occur
	targetPlayerX := originalPlayerX + dx_frame
	targetPlayerY := originalPlayerY + dy_frame

	// --- Player-Obstacle Collision Resolution ---
	// We'll try to move along X, then along Y, to handle collisions.

	// Assume player's X will be targetPlayerX unless collision
	newPlayerX := targetPlayerX

	// Check X-axis collision:
	// Create a bounding box for the player at (targetPlayerX, originalPlayerY)
	playerBoundingBoxX := image.Rect(
		int(targetPlayerX-g.player.CollisionRadius),
		int(originalPlayerY-g.player.CollisionRadius),
		int(targetPlayerX+g.player.CollisionRadius),
		int(originalPlayerY+g.player.CollisionRadius),
	)

	for _, obs := range g.obstacles {
		obsRect := image.Rect(int(obs.X), int(obs.Y), int(obs.X+obs.Width), int(obs.Y+obs.Height))
		if playerBoundingBoxX.Overlaps(obsRect) {
			newPlayerX = originalPlayerX // Collision on X: revert X movement
			break
		}
	}
	g.player.X = newPlayerX // Commit X position (either target or original)

	// Assume player's Y will be targetPlayerY unless collision
	newPlayerY := targetPlayerY

	// Check Y-axis collision:
	// Create a bounding box for the player at (g.player.X (which is newPlayerX), targetPlayerY)
	playerBoundingBoxY := image.Rect(
		int(g.player.X-g.player.CollisionRadius),    // Use the already resolved X
		int(targetPlayerY-g.player.CollisionRadius),
		int(g.player.X+g.player.CollisionRadius),    // Use the already resolved X
		int(targetPlayerY+g.player.CollisionRadius),
	)

	for _, obs := range g.obstacles {
		obsRect := image.Rect(int(obs.X), int(obs.Y), int(obs.X+obs.Width), int(obs.Y+obs.Height))
		if playerBoundingBoxY.Overlaps(obsRect) {
			newPlayerY = originalPlayerY // Collision on Y: revert Y movement
			break
		}
	}
	g.player.Y = newPlayerY // Commit Y position

	// Keep player within world bounds
	// Player's X, Y is center, so adjust bounds by CollisionRadius.
	// This clamping is done *after* obstacle collision resolution.
	g.player.X = clamp(g.player.X, g.player.CollisionRadius, worldWidth-g.player.CollisionRadius)
	g.player.Y = clamp(g.player.Y, g.player.CollisionRadius, worldHeight-g.player.CollisionRadius)

	// Camera Logic: Smoothly follow the player
	// Target camera position to center the player on screen
	targetCamX := g.player.X - screenWidth/2
	targetCamY := g.player.Y - screenHeight/2

	// Lerp camera towards target position
	g.camX += (targetCamX - g.camX) * cameraLerpSpeed
	g.camY += (targetCamY - g.camY) * cameraLerpSpeed

	// Clamp camera to world boundaries
	// Prevents camera from showing areas outside the defined world.
	// Max camera X is worldWidth - screenWidth, so the right edge of camera view aligns with world right edge.
	// Similar for Y.
	g.camX = clamp(g.camX, 0, worldWidth-screenWidth)
	g.camY = clamp(g.camY, 0, worldHeight-screenHeight)


	// Attack Logic: Player's automatic pulse attack
	// The attackTimer accumulates time. When it exceeds pulseAttackCooldown (1 second),
	// a new attack is spawned, and the timer resets.
	g.attackTimer += 1.0 / float64(ebiten.TPS()) // ebiten.TPS() gives ticks per second.
	if g.attackTimer >= pulseAttackCooldown {
		g.attackTimer = 0 // Reset timer
		// Create a new pulse attack at player's current location
		newAttack := &PulseAttack{
			X:              g.player.X,
			Y:              g.player.Y,
			Radius:         0,
			MaxRadius:      pulseAttackMaxRadius,
			ExpansionSpeed: pulseAttackExpansionSpeed,
			Image:          pulseAttackImage, // Using the base 1x1 image
		}
		g.pulseAttacks = append(g.pulseAttacks, newAttack)
	}

	// Update existing pulse attacks
	// We will handle removal in the cleanup step later, for now just update
	for _, attack := range g.pulseAttacks {
		attack.Radius += attack.ExpansionSpeed / float64(ebiten.TPS())
	}

	// Enemy Spawning Logic
	// The spawnTimer accumulates time. When it exceeds g.enemySpawnRate (e.g., 3 seconds),
	// a new wave of enemies is spawned, and the timer resets.
	// Enemies are spawned randomly along the edges, just outside the screen view.
	g.spawnTimer += 1.0 / float64(ebiten.TPS())
	if g.spawnTimer >= g.enemySpawnRate {
		g.spawnTimer = 0 // Reset spawn timer
		// Example: Increase difficulty over time (optional)
		// if g.enemiesPerWave < 20 { g.enemiesPerWave++ }
		// if g.enemySpawnRate > 1.0 { g.enemySpawnRate *= 0.99 }


		for i := 0; i < g.enemiesPerWave; i++ {
			// Spawn enemies randomly off-screen from one of the four sides.
			const spawnMargin = 50 // Defines how far off-screen from current view enemies will spawn.
			var ex, ey float64

			// Determine spawn position relative to camera view
			side := rand.Intn(4) // 0: top, 1: bottom, 2: left, 3: right
			switch side {
			case 0: // Top, relative to camera view
				ex = g.camX + rand.Float64()*screenWidth
				ey = g.camY - spawnMargin
			case 1: // Bottom, relative to camera view
				ex = g.camX + rand.Float64()*screenWidth
				ey = g.camY + screenHeight + spawnMargin
			case 2: // Left, relative to camera view
				ex = g.camX - spawnMargin
				ey = g.camY + rand.Float64()*screenHeight
			case 3: // Right, relative to camera view
				ex = g.camX + screenWidth + spawnMargin
				ey = g.camY + rand.Float64()*screenHeight
			}

			// Clamp spawn position to world boundaries
			// Enemy's X,Y is center, so consider its collision radius for clamping to world edge.
			ex = clamp(ex, enemyCollisionRadius, worldWidth-enemyCollisionRadius)
			ey = clamp(ey, enemyCollisionRadius, worldHeight-enemyCollisionRadius)

			// Additional check: if after clamping, the enemy is now on-screen due to camera being at edge,
			// try to push it further. This is a bit tricky. For now, the clamping might suffice if spawnMargin is large enough.
			// A more robust solution would be to pick a point on a wider perimeter around the camera.

			newEnemy := &Enemy{
				X:     ex,
				Y:     ey,
				Speed: enemySpeed,
				Health: enemyHealth,
				Image: enemyImage,
				CollisionRadius: enemyCollisionRadius,
			}
			g.enemies = append(g.enemies, newEnemy)
		}
	}

	// Update Enemies: Movement AI
	for _, enemy := range g.enemies {
		// Calculate vector from enemy to player
		dx := g.player.X - enemy.X
		dy := g.player.Y - enemy.Y

		// Normalize the vector (dx, dy) to get a unit vector representing the direction.
		// This ensures enemies move at a consistent speed regardless of distance to player.
		// math.Hypot(dx, dy) calculates the length (magnitude) of the vector.
		normalizedDx, normalizedDy := normalizeVector(dx, dy)

		// Move enemy towards player based on its speed and the normalized direction.
		potentialEnemyX := enemy.X + normalizedDx*enemy.Speed/float64(ebiten.TPS())
		potentialEnemyY := enemy.Y + normalizedDy*enemy.Speed/float64(ebiten.TPS())

		// Enemy-Obstacle Collision (similar to player)
		// Check X-axis
		enemyRectX := image.Rect(
			int(potentialEnemyX-enemy.CollisionRadius),
			int(enemy.Y-enemy.CollisionRadius),
			int(potentialEnemyX+enemy.CollisionRadius),
			int(enemy.Y+enemy.CollisionRadius),
		)
		collidedX := false
		for _, obs := range g.obstacles {
			obsRect := image.Rect(int(obs.X), int(obs.Y), int(obs.X+obs.Width), int(obs.Y+obs.Height))
			if enemyRectX.Overlaps(obsRect) {
				collidedX = true
				break
			}
		}
		if !collidedX {
			enemy.X = potentialEnemyX
		}

		// Check Y-axis (using updated enemy.X if it changed, or original if X was blocked)
		currentEnemyXForYCheck := enemy.X // Use the (potentially reverted) X
		enemyRectY := image.Rect(
			int(currentEnemyXForYCheck-enemy.CollisionRadius),
			int(potentialEnemyY-enemy.CollisionRadius),
			int(currentEnemyXForYCheck+enemy.CollisionRadius),
			int(potentialEnemyY+enemy.CollisionRadius),
		)
		collidedY := false
		for _, obs := range g.obstacles {
			obsRect := image.Rect(int(obs.X), int(obs.Y), int(obs.X+obs.Width), int(obs.Y+obs.Height))
			if enemyRectY.Overlaps(obsRect) {
				collidedY = true
				break
			}
		}
		if !collidedY {
			enemy.Y = potentialEnemyY
		}
	}

	// Collision Detection Logic

	// Attack-Enemy Collision: Check if any pulse attack hits any enemy.
	for _, attack := range g.pulseAttacks {
		// Only consider attacks that are still expanding.
		if attack.Radius <= attack.MaxRadius {
			for _, enemy := range g.enemies {
				if enemy.Health > 0 { // Only interact with living enemies.
					dist := distance(attack.X, attack.Y, enemy.X, enemy.Y)
					// Collision occurs if the distance between the attack's center and the enemy's center
					// is less than the sum of the attack's current radius and the enemy's collision radius.
					if dist <= attack.Radius+enemy.CollisionRadius {
						enemy.Health-- // Damage the enemy.
						if enemy.Health <= 0 {
							g.score += 10 // Award score for defeating an enemy.

							// Spawn an XP Orb
							orb := &XPOrb{
								X:               enemy.X,
								Y:               enemy.Y,
								Value:           xpOrbValue,
								Image:           xpOrbImage,
								CollisionRadius: xpOrbCollisionRadius,
								Collected:       false,
							}
							g.xpOrbs = append(g.xpOrbs, orb)
							// The enemy will be removed from the game in the cleanup phase.
						}
					}
				}
			}
		}
	}

	// Player-Enemy Collision: Check if any enemy collides with the player.
	for _, enemy := range g.enemies {
		if enemy.Health > 0 { // Only living enemies can collide.
			dist := distance(g.player.X, g.player.Y, enemy.X, enemy.Y)
			if dist < g.player.CollisionRadius+enemy.CollisionRadius {
				if g.player.invulnerabilityTimer <= 0 {
					g.player.CurrentHealth -= enemyContactDamage
					g.player.invulnerabilityTimer = playerHitInvulnerabilityDuration
					// Death check will happen separately after all updates for the frame
				}
				// No break here, as multiple enemies could hit in theory, though invulnerability handles it.
				// However, if an enemy hits, it's usually one interaction per frame of collision logic.
				// For simplicity, one hit processing per frame of collision loop is fine.
				// If player dies from this hit, gameOver will be set later.
			}
		}
	}

	// Player-XPOrb Collision (Collection)
	for _, orb := range g.xpOrbs {
		if !orb.Collected {
			dist := distance(g.player.X, g.player.Y, orb.X, orb.Y)
			// Player's collision radius + orb's collision radius
			if dist < g.player.CollisionRadius+orb.CollisionRadius {
				g.player.CurrentXP += orb.Value
				orb.Collected = true // Mark for removal
				// Level up check will happen after all XP is collected in this frame
			}
		}
	}

	// Level Up Check (can happen multiple times if enough XP is gained at once)
	for g.player.CurrentXP >= g.player.XPToNextLevel {
		g.player.Level++
		g.player.CurrentXP -= g.player.XPToNextLevel // Subtract cost of current level, carry over excess
		g.player.XPToNextLevel = calculateXPForLevel(g.player.Level)
		log.Printf("Player reached Level %d! Next level in %d XP.", g.player.Level, g.player.XPToNextLevel)
		g.levelUpMessageTimer = levelUpMessageDuration // Display "LEVEL UP!" message
	}

	// Decrement Timers
	if g.levelUpMessageTimer > 0 {
		g.levelUpMessageTimer -= 1.0 / float64(ebiten.TPS())
		if g.levelUpMessageTimer < 0 {
			g.levelUpMessageTimer = 0
		}
	}
	if g.player.invulnerabilityTimer > 0 {
		g.player.invulnerabilityTimer -= 1.0 / float64(ebiten.TPS())
		if g.player.invulnerabilityTimer < 0 {
			g.player.invulnerabilityTimer = 0
		}
	}

	// Player Death Check
	if g.player.CurrentHealth <= 0 {
		g.currentState = StateLoseScreen // Transition to Lose Screen
		return // Stop further gameplay updates for this frame
	}

	// Entity Cleanup: Remove "dead" entities.
	// This section should always run if in gameplay, regardless of g.gameOver (which is removed)
	// The transition to StateLoseScreen effectively stops gameplay logic for next frame.
	// Cleanup Pulse Attacks that have exceeded their MaxRadius.
	activeAttacks := make([]*PulseAttack, 0, len(g.pulseAttacks))
	for _, attack := range g.pulseAttacks {
			if attack.Radius <= attack.MaxRadius { // Keep only attacks within their effective radius.
				activeAttacks = append(activeAttacks, attack)
			}
		}
		g.pulseAttacks = activeAttacks

		// Cleanup defeated Enemies (Health <= 0).
		aliveEnemies := make([]*Enemy, 0, len(g.enemies))
		for _, enemy := range g.enemies {
			if enemy.Health > 0 { // Keep only enemies with health above 0.
				aliveEnemies = append(aliveEnemies, enemy)
			}
		}
		g.enemies = aliveEnemies

		// Cleanup collected XPOrbs
		activeOrbs := make([]*XPOrb, 0, len(g.xpOrbs))
		for _, orb := range g.xpOrbs {
			if !orb.Collected {
				activeOrbs = append(activeOrbs, orb)
			}
		}
		g.xpOrbs = activeOrbs
	}

	// return nil // updateGameplayScreen does not return error
}

func (g *Game) updateLoseScreen() {
	if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
		g.loseSelectedOption--
		if g.loseSelectedOption < 0 {
			g.loseSelectedOption = 1 // Wrap around (0: Retry, 1: Main Menu)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
		g.loseSelectedOption++
		if g.loseSelectedOption > 1 {
			g.loseSelectedOption = 0 // Wrap around
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		switch g.loseSelectedOption {
		case 0: // Retry
			g.reset() // Reset game state
			g.currentState = StateGameplay
		case 1: // Main Menu
			// Optionally reset selected option for title screen if needed, or let it persist
			g.titleSelectedOption = 0
			g.currentState = StateTitleScreen
		}
	}
}

// Draw draws the game screen.
// Draw is called every frame (typically 1/60[s] for 60Hz display).
func (g *Game) Draw(screen *ebiten.Image) {
	switch g.currentState {
	case StateTitleScreen:
		g.drawTitleScreen(screen)
	case StateGameplay:
		g.drawGameplayScreen(screen)
	case StateLoseScreen:
		g.drawLoseScreen(screen)
	}
}

func (g *Game) drawTitleScreen(screen *ebiten.Image) {
	screen.Fill(color.NRGBA{R: 20, G: 20, B: 40, A: 255}) // Dark blue background for title

	titleText := "Go Survivor"
	titleTextWidth := len(titleText) * 6 // Approximate width
	ebitenutil.DebugPrintAt(screen, titleText, screenWidth/2-titleTextWidth*2, screenHeight/4) // Larger text by spacing

	startText := "Start Game"
	quitText := "Quit"

	if g.titleSelectedOption == 0 {
		startText = "> " + startText
	} else {
		quitText = "> " + quitText
	}

	ebitenutil.DebugPrintAt(screen, startText, screenWidth/2-50, screenHeight/2)
	ebitenutil.DebugPrintAt(screen, quitText, screenWidth/2-50, screenHeight/2+30)

	instructions := "Use Up/Down Arrows & Enter"
	ebitenutil.DebugPrintAt(screen, instructions, screenWidth/2-len(instructions)*3, screenHeight-50)
}

func (g *Game) drawGameplayScreen(screen *ebiten.Image) {
	// This will contain the existing core game draw logic
	// Optional: Fill background
	// screen.Fill(color.NRGBA{R: 10, G: 10, B: 30, A: 255})

	// Draw Obstacles
	for _, obs := range g.obstacles {
		if obs.Image != nil {
			opts := &ebiten.DrawImageOptions{}
			// Obstacle X,Y is top-left for its image.
			opts.GeoM.Translate(obs.X, obs.Y)
			// Apply camera view
			opts.GeoM.Translate(-g.camX, -g.camY)
			screen.DrawImage(obs.Image, opts)
		}
	}

	// Draw Player
	if g.player != nil && g.player.Image != nil {
		opts := &ebiten.DrawImageOptions{}
		// Player X,Y is center. Translate image to be centered.
		opts.GeoM.Translate(-float64(g.player.Image.Bounds().Dx())/2, -float64(g.player.Image.Bounds().Dy())/2)
		opts.GeoM.Translate(g.player.X, g.player.Y)
		// Apply camera view
		opts.GeoM.Translate(-g.camX, -g.camY)

		// Player invulnerability visual feedback (flashing)
		if g.player.invulnerabilityTimer > 0 {
			// Blink effect: alternate alpha rapidly
			// Check if the fractional part of timer*5 (e.g., every 0.2s cycle for timer*5) is in the "off" phase
			if math.Mod(g.player.invulnerabilityTimer*10, 2) > 1 { // Blink rapidly
				opts.ColorScale.ScaleAlpha(0.5) // Make player semi-transparent
			}
		}
		screen.DrawImage(g.player.Image, opts)

		// Draw Player's Health Bar (above player, world space)
		if g.player.CurrentHealth > 0 { // Only draw if alive (though player disappears on death/reset)
			playerSpriteWidth := float64(g.player.Image.Bounds().Dx())
			healthBarFullWidth := playerSpriteWidth * 1.2 // Slightly wider than player
			healthBarHeight := 5.0
			healthBarGap := 10.0 // Gap above player's sprite top edge

			// Position of the health bar's top-left corner in world coordinates
			barWorldX := g.player.X - healthBarFullWidth/2
			barWorldY := g.player.Y - g.player.CollisionRadius - healthBarGap - healthBarHeight

			// Health ratio
			healthRatio := 0.0
			if g.player.MaxHealth > 0 {
				healthRatio = float64(g.player.CurrentHealth) / float64(g.player.MaxHealth)
			}
			currentHealthWidth := healthRatio * healthBarFullWidth
			if currentHealthWidth < 0 { currentHealthWidth = 0 }

			// Background for health bar (dark red or gray)
			bgOpts := &ebiten.DrawImageOptions{}
			// Create a 1x1 image for drawing rects if needed, or use ebitenutil.DrawRect equivalent for world space
			// For world space rects, it's easier to use a 1x1 image and scale/color it.
			// Let's use opaqueClearImage (1x1 white) and color it.
			bgOpts.GeoM.Scale(healthBarFullWidth, healthBarHeight)
			bgOpts.GeoM.Translate(barWorldX, barWorldY)
			bgOpts.GeoM.Translate(-g.camX, -g.camY) // Apply camera
			bgOpts.ColorScale.Scale(0.3, 0.3, 0.3, 1) // Dark Gray
			screen.DrawImage(opaqueClearImage, bgOpts)

			// Foreground for health bar (green or bright red)
			fgOpts := &ebiten.DrawImageOptions{}
			fgOpts.GeoM.Scale(currentHealthWidth, healthBarHeight)
			fgOpts.GeoM.Translate(barWorldX, barWorldY) // Position is the same as background
			fgOpts.GeoM.Translate(-g.camX, -g.camY) // Apply camera
			fgOpts.ColorScale.Scale(0.8, 0.2, 0.2, 1) // Red
			if healthRatio > 0.6 { // Green if high health
				fgOpts.ColorScale.Reset()
				fgOpts.ColorScale.Scale(0.2, 0.8, 0.2, 1)
			} else if healthRatio > 0.3 { // Yellow if medium health
				fgOpts.ColorScale.Reset()
				fgOpts.ColorScale.Scale(0.8, 0.8, 0.2, 1)
			}
			screen.DrawImage(opaqueClearImage, fgOpts)
		}
	}

	// Draw Pulse Attacks
	for _, attack := range g.pulseAttacks {
		if attack.Radius > 0 && attack.Image != nil {
			// Outer ring
			outerOpts := &ebiten.DrawImageOptions{}
			// Image is 1x1, scale to diameter, then position.
			// Origin for scaling is top-left of the 1x1 image.
			outerOpts.GeoM.Scale(attack.Radius*2, attack.Radius*2)
			// Translate scaled image so its center is at attack.X, attack.Y
			outerOpts.GeoM.Translate(attack.X-attack.Radius, attack.Y-attack.Radius)
			// Apply camera view
			outerOpts.GeoM.Translate(-g.camX, -g.camY)
			screen.DrawImage(attack.Image, outerOpts)

			// Inner cutout
			innerRadius := attack.Radius - pulseRingThickness
			if innerRadius > 0 {
				innerOpts := &ebiten.DrawImageOptions{}
				innerOpts.GeoM.Scale(innerRadius*2, innerRadius*2)
				innerOpts.GeoM.Translate(attack.X-innerRadius, attack.Y-innerRadius)
				// Apply camera view
				innerOpts.GeoM.Translate(-g.camX, -g.camY)
				innerOpts.CompositeMode = ebiten.CompositeModeClear
				screen.DrawImage(opaqueClearImage, innerOpts)
			}
		}
	}

	// Draw Enemies
	for _, enemy := range g.enemies {
		if enemy.Image != nil {
			opts := &ebiten.DrawImageOptions{}
			// Enemy X,Y is center.
			opts.GeoM.Translate(-float64(enemy.Image.Bounds().Dx())/2, -float64(enemy.Image.Bounds().Dy())/2)
			opts.GeoM.Translate(enemy.X, enemy.Y)
			// Apply camera view
			opts.GeoM.Translate(-g.camX, -g.camY)
			screen.DrawImage(enemy.Image, opts)
		}
	}

	// Draw XPOrbs (relative to camera)
	for _, orb := range g.xpOrbs {
		if !orb.Collected && orb.Image != nil {
			opts := &ebiten.DrawImageOptions{}
			// Orb X,Y is center.
			opts.GeoM.Translate(-float64(orb.Image.Bounds().Dx())/2, -float64(orb.Image.Bounds().Dy())/2)
			opts.GeoM.Translate(orb.X, orb.Y)
			// Apply camera view
			opts.GeoM.Translate(-g.camX, -g.camY)
			screen.DrawImage(orb.Image, opts)
		}
	}

	// --- UI Elements (drawn in screen space, not affected by camera) ---

	// Draw XP Bar
	xpBarWidth := screenWidth - 40 // Bar width with some padding
	xpBarHeight := 20
	xpBarX := (screenWidth - xpBarWidth) / 2
	xpBarY := 10

	// Background of XP bar
	ebitenutil.DrawRect(screen, float64(xpBarX), float64(xpBarY), float64(xpBarWidth), float64(xpBarHeight), color.Gray{Y: 50})

	// Foreground of XP bar (current XP)
	xpRatio := 0.0
	if g.player.XPToNextLevel > 0 { // Avoid division by zero if XPToNextLevel is somehow 0
		xpRatio = float64(g.player.CurrentXP) / float64(g.player.XPToNextLevel)
	}
	currentXPWidth := xpRatio * float64(xpBarWidth)
	ebitenutil.DrawRect(screen, float64(xpBarX), float64(xpBarY), currentXPWidth, float64(xpBarHeight), color.RGBA{R: 100, G: 200, B: 100, A: 255}) // Greenish

	// Draw Level Text
	levelText := "Level: " + strconv.Itoa(g.player.Level)
	ebitenutil.DebugPrintAt(screen, levelText, xpBarX, xpBarY+xpBarHeight+5) // Stays in screen space

	// Display Score
	scoreText := "Score: " + strconv.Itoa(g.score)
	ebitenutil.DebugPrintAt(screen, scoreText, screenWidth-150, xpBarY+xpBarHeight+5) // Near top right

	// Display "LEVEL UP!" message
	if g.levelUpMessageTimer > 0 {
		levelUpText := "LEVEL UP!"
		// For "large text" with DebugPrint, we can't scale. We just center it.
		// True large text would require loading fonts and using text.Draw.
		textWidth := len(levelUpText) * 6 // Approximate width for DebugPrint characters
		ebitenutil.DebugPrintAt(screen, levelUpText, screenWidth/2-textWidth/2, screenHeight/2-10)
	}
}

func (g *Game) drawLoseScreen(screen *ebiten.Image) {
	screen.Fill(color.NRGBA{R: 50, G: 20, B: 20, A: 255}) // Dark red background for lose screen

	loseText := "YOU DIED"
	textWidth := len(loseText) * 6 // Approximate width
	ebitenutil.DebugPrintAt(screen, loseText, screenWidth/2-textWidth*2, screenHeight/4) // Larger text by spacing

	finalScoreText := "Final Score: " + strconv.Itoa(g.score)
	ebitenutil.DebugPrintAt(screen, finalScoreText, screenWidth/2-len(finalScoreText)*3, screenHeight/4+40)

	finalLevelText := "Level Reached: " + strconv.Itoa(g.player.Level)
	ebitenutil.DebugPrintAt(screen, finalLevelText, screenWidth/2-len(finalLevelText)*3, screenHeight/4+60)


	retryText := "Retry"
	menuText := "Main Menu"

	if g.loseSelectedOption == 0 {
		retryText = "> " + retryText
	} else {
		menuText = "> " + menuText
	}

	ebitenutil.DebugPrintAt(screen, retryText, screenWidth/2-50, screenHeight/2+30)
	ebitenutil.DebugPrintAt(screen, menuText, screenWidth/2-50, screenHeight/2+60)

	instructions := "Use Up/Down Arrows & Enter"
	ebitenutil.DebugPrintAt(screen, instructions, screenWidth/2-len(instructions)*3, screenHeight-50)
}


// Layout takes the outside size (e.g., window size) and returns the (logical) screen size.
// If you don't have to adjust the screen size with the outside size, just return a fixed size.
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Go Survivor")

	game := NewGame()

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

// distance calculates the Euclidean distance between two points (x1,y1) and (x2,y2).
func distance(x1, y1, x2, y2 float64) float64 {
	return math.Sqrt(math.Pow(x2-x1, 2) + math.Pow(y2-y1, 2))
}

// normalizeVector takes a 2D vector (x,y) and returns its normalized form (unit vector).
// A unit vector has a magnitude (length) of 1, representing pure direction.
// If the input vector has a length of 0, it returns (0,0) to avoid division by zero.
func normalizeVector(x, y float64) (float64, float64) {
	length := math.Hypot(x, y) // math.Hypot calculates Sqrt(x*x + y*y)
	if length == 0 {
		return 0, 0 // Avoid division by zero if the vector is (0,0).
	}
	return x / length, y / length // Divide each component by the length.
}

// clamp restricts a value to be within a specified range [min, max].
func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
