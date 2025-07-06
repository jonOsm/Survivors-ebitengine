package main

import (
	"image" // Added for image.Rect
	"fmt" // For Sprintf in dev interface
	"image/color"
	"log"
	"math"
	"math/rand"
	"os" // For os.Exit
	"sort" // For sorting enemies by distance
	"strconv"
	"strings" // For text wrapping
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
	magicMissileImage *ebiten.Image // Image for magic missiles
)

// GameState defines the current state of the game.
type GameState int

const (
	StateTitleScreen GameState = iota
	StateGameplay
	StateLoseScreen
	StateLevelUpSelection // New state for choosing power-ups
)

// Card dimensions and layout constants for level up screen
const (
	levelUpCardWidth      = float64(screenWidth / 3) // Width of a single card
	levelUpCardHeight     = float64(screenHeight / 2) // Height of a single card
	levelUpCardSpacing    = 20.0                     // Horizontal space between cards
	levelUpCardPadding    = 15.0                     // Padding inside the card for text
	levelUpTitleOffsetY   = 20.0
	levelUpDescOffsetY    = 50.0
	levelUpOptionMaxChars = 30 // Approx chars per line for description wrapping
)


// PowerUpDefinition defines the properties of a power-up.
type PowerUpDefinition struct {
	ID          string
	Title       string
	Description string
}

var availablePowerUps = []PowerUpDefinition{
	{"PUP001", "Speed Boost", "Slightly increases player movement speed."},
	{"PUP002", "Attack Speed Up", "Player attacks slightly faster."},
	{"PUP003", "Increased XP Gain", "Gain more XP from orbs."},
	{"PUP004", "Larger Attack Radius", "Pulse attack maximum radius increased."},
	{"PUP005", "Extra Health", "Increases Max Health by a small amount."},
	{"PUP006", "Magic Missile", "Fires a homing missile every 2s that weaves and deals 2 damage."},
	{"PUP007", "Twin Barrage", "Magic Missile fires an additional projectile."},
	{"PUP008", "Phantom Edge", "Magic Missile pierces 1 additional enemy."}, // Restored
}

// MagicMissile represents a homing, weaving projectile.
type MagicMissile struct {
	X, Y           float64
	Speed          float64
	Damage         int
	TargetEnemy    *Enemy // Could be nil if target dies
	CurrentAngle   float64 // For rotation and possibly movement logic
	timeAlive      float64 // For weaving pattern and lifetime
	Image          *ebiten.Image
	ToRemove       bool
	MaxPierces     int             // Max number of enemies this missile can pierce (restored)
	PiercesMade    int             // How many enemies this missile has pierced so far (restored)
	hitTargets     map[*Enemy]bool // Tracks enemies already hit by this specific missile instance (restored)
}

// Game implements ebiten.Game interface.
type Game struct {
	player                   *Player
	enemies                  []*Enemy
	pulseAttacks             []*PulseAttack
	magicMissiles            []*MagicMissile // New slice for magic missiles
	attackTimer              float64 // Seconds
	spawnTimer               float64 // Seconds
	score                    int
	enemySpawnRate           float64 // Seconds between enemy spawns
	enemiesPerWave           int
	camX, camY               float64
	obstacles                []*Obstacle
	xpOrbs                   []*XPOrb // Slice to hold active XP orbs
	// levelUpMessageTimer      float64  // No longer used, state transition handles level up UI

	currentState             GameState
	titleSelectedOption      int // 0 for Start, 1 for Quit
	loseSelectedOption       int // 0 for Retry, 1 for Main Menu
	levelUpSelectedCardIndex int // 0 or 1 for the two choices
	currentPowerUpChoices    [2]*PowerUpDefinition
	showDevInterface         bool   // Toggled by F2
	levelUpSelectionInputMode string // "keyboard" or "mouse"
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
	AcquiredPowerUps []string // Stores IDs of acquired power-ups

	// Modifiable stats by power-ups
	AttackCooldown   float64 // Current attack cooldown in seconds
	AttackMaxRadius  float64 // Current max radius for pulse attack
	XPMultiplier     float64 // Multiplier for XP gained

	// Magic Missile specific
	HasMagicMissile         bool
	magicMissileFireTimer   float64 // Cooldown timer for firing missiles
	MagicMissileCount       int     // Number of missiles to fire at once
	MagicMissilePiercing    int     // How many additional targets missiles can pierce (restored)
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
	MaxRadius      float64 // This will now be set from player's stat when attack is created
	ExpansionSpeed float64
	Image          *ebiten.Image // This will be dynamically resized or redrawn
}

// Base player and attack stats (before power-ups)
const (
	basePlayerSpeed           = 200.0
	basePulseAttackCooldown   = 1.0   // seconds
	basePulseAttackMaxRadius  = 32.0
	pulseAttackExpansionSpeed = 40    // pixels per second (remains constant for now)
	pulseRingThickness        = 4     // Thickness of the pulse ring in pixels.

	magicMissileCooldown = 2.0   // seconds
	magicMissileSpeed    = 180.0 // pixels per second
	magicMissileDamage   = 2     // Pulse effectively does 1 damage to 1-HP enemies
	magicMissileLifetime = 4.0   // seconds
	magicMissileWeaveFrequency = 5.0 // Radians per second for oscillation
	magicMissileWeaveMagnitude = 20.0 // Sideways pixels

	xpOrbValue                = 25    // XP gained per orb
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

	magicMissileImage = ebiten.NewImage(8, 8)
	// Simple pointed shape (approximate triangle pointing right)
	// For a real game, use a proper sprite.
	// This will be a small square for now, rotation will make it look more dynamic.
	magicMissileImage.Fill(color.RGBA{R: 0, G: 255, B: 255, A: 255}) // Cyan

	rand.Seed(time.Now().UnixNano())
}

// NewGame initializes a new game.
func NewGame() *Game {
	g := &Game{
		player: &Player{
			X:               worldWidth / 2,  // Start player at world center
			Y:               worldHeight / 2, // Start player at world center
			// Speed:           200, // Pixels per second // Removed duplicate
			Image:           playerImage,
			CollisionRadius: playerCollisionRadius,
			Level:           0,
			CurrentXP:       0,
			XPToNextLevel:   calculateXPForLevel(0),
			MaxHealth:       playerMaxHealth,
			CurrentHealth:   playerMaxHealth,
			invulnerabilityTimer: 0,
			Speed:           basePlayerSpeed, // Initialize with base value
			AttackCooldown:  basePulseAttackCooldown,
			AttackMaxRadius: basePulseAttackMaxRadius,
			XPMultiplier:    1.0,
			HasMagicMissile: false,
			magicMissileFireTimer: 0,
			MagicMissileCount:    0, // Will be set to 1 when PUP006 is acquired
			MagicMissilePiercing: 0, // Restored
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
		// levelUpMessageTimer: 0, // Removed
		currentState:        StateTitleScreen,
		titleSelectedOption: 0, // Default to "Start Game"
		loseSelectedOption:  0, // Default to "Retry"
		levelUpSelectedCardIndex: 0,
		currentPowerUpChoices: [2]*PowerUpDefinition{nil, nil}, // Initialize with nils
		showDevInterface:         false,
		levelUpSelectionInputMode: "keyboard", // Default to keyboard, or set in prepareLevelUpChoices
		magicMissiles:            []*MagicMissile{},
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

	// Reset level and XP
	g.player.Level = 0
	g.player.CurrentXP = 0
	g.player.XPToNextLevel = calculateXPForLevel(0)
	g.player.MaxHealth = playerMaxHealth
	g.player.CurrentHealth = playerMaxHealth
	g.player.invulnerabilityTimer = 0
	g.player.AcquiredPowerUps = []string{} // Clear acquired power-ups
	g.player.Speed = basePlayerSpeed
	g.player.AttackCooldown = basePulseAttackCooldown
	g.player.AttackMaxRadius = basePulseAttackMaxRadius
	g.player.XPMultiplier = 1.0
	g.player.HasMagicMissile = false
	g.player.magicMissileFireTimer = 0
	g.player.MagicMissileCount = 0
	g.player.MagicMissilePiercing = 0 // Restored


	// Center camera on player
	g.camX = g.player.X - screenWidth/2
	g.camY = g.player.Y - screenHeight/2
	g.camX = clamp(g.camX, 0, worldWidth-screenWidth)
	g.camY = clamp(g.camY, 0, worldHeight-screenHeight)


	g.enemies = []*Enemy{}      // Clear enemies
	g.pulseAttacks = []*PulseAttack{} // Clear attacks
	g.xpOrbs = []*XPOrb{}       // Clear XP orbs
	g.magicMissiles = []*MagicMissile{} // Clear magic missiles

	g.attackTimer = 0
	g.spawnTimer = 0 // Reset spawn timer to allow immediate first wave on reset
	g.score = 0
	// g.gameOver = false // No longer needed here, state transitions handle flow
	// g.levelUpMessageTimer = 0 // Removed

	// Reset wave progression if it was dynamic (using defaults here)
	g.enemySpawnRate = defaultEnemySpawnRate
	g.enemiesPerWave = defaultEnemiesPerWave
}

// Update proceeds the game state.
// Update is called every tick (1/60 [s] by default).
func (g *Game) Update() error {
	// Global input checks (like dev interface toggle)
	if inpututil.IsKeyJustPressed(ebiten.KeyF2) {
		g.showDevInterface = !g.showDevInterface
	}

	switch g.currentState {
	case StateTitleScreen:
		g.updateTitleScreen()
	case StateGameplay:
		g.updateGameplayScreen()
	case StateLevelUpSelection:
		g.updateLevelUpSelectionScreen()
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
	// The attackTimer accumulates time. When it exceeds player's AttackCooldown,
	// a new attack is spawned, and the timer resets.
	g.attackTimer += 1.0 / float64(ebiten.TPS()) // ebiten.TPS() gives ticks per second.
	if g.attackTimer >= g.player.AttackCooldown { // Use player's current attack cooldown
		g.attackTimer = 0 // Reset timer
		// Create a new pulse attack at player's current location
		newAttack := &PulseAttack{
			X:              g.player.X,
			Y:              g.player.Y,
			Radius:         0,
			MaxRadius:      g.player.AttackMaxRadius, // Use player's current attack max radius
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
				xpGained := float64(orb.Value) * g.player.XPMultiplier
				g.player.CurrentXP += int(xpGained)
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

		g.prepareLevelUpChoices()
		g.currentState = StateLevelUpSelection
		// No need to decrement levelUpMessageTimer here as we are transitioning state.
		// The old on-screen "LEVEL UP!" message is replaced by the selection screen.
		return // Exit updateGameplayScreen to prevent further game logic this frame
	}

	// Decrement Timers
	// The g.levelUpMessageTimer is now vestigial and its logic/field can be removed.
	// if g.levelUpMessageTimer > 0 {
	// 	g.levelUpMessageTimer -= 1.0 / float64(ebiten.TPS())
	// 	if g.levelUpMessageTimer < 0 {
	// 		g.levelUpMessageTimer = 0
	// 	}
	// }
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

		// Cleanup removed Magic Missiles
		activeMagicMissiles := make([]*MagicMissile, 0, len(g.magicMissiles))
		for _, m := range g.magicMissiles {
			if !m.ToRemove {
				activeMagicMissiles = append(activeMagicMissiles, m)
			}
		}
		g.magicMissiles = activeMagicMissiles
	// } // This was the end of the old 'if !g.gameOver' block, now removed.

	// Update Magic Missiles
	for _, m := range g.magicMissiles {
		if m.ToRemove {
			continue
		}

		m.timeAlive += 1.0 / float64(ebiten.TPS())
		if m.timeAlive > magicMissileLifetime {
			m.ToRemove = true
			continue
		}

		// Target Loss Logic (Adjusted for Pierce)
		if m.TargetEnemy != nil && m.TargetEnemy.Health <= 0 { // Target died
			m.TargetEnemy = nil // Stop homing, missile will fly straight
			// If it couldn't pierce (MaxPierces = 0), it might have been removed on hit already.
			// If it *can* pierce, it continues straight and relies on opportunistic pierce or lifetime.
		}
		// Note: if m.TargetEnemy was nil initially (e.g. directional shot), it remains nil.

		// Homing and Weaving Movement
		var dirToTargetX, dirToTargetY float64
		if m.TargetEnemy != nil { // Only home if there's a live target
			targetX, targetY := m.TargetEnemy.X, m.TargetEnemy.Y
			dirToTargetX, dirToTargetY = normalizeVector(targetX-m.X, targetY-m.Y)
			m.CurrentAngle = math.Atan2(dirToTargetY, dirToTargetX) // Update angle towards target
		} else {
			// No target, fly straight using current angle (set at launch or last known target direction)
			dirToTargetX = math.Cos(m.CurrentAngle)
			dirToTargetY = math.Sin(m.CurrentAngle)
		}

		// Weaving is always applied relative to the (potentially straight) direction
		oscillationFactor := math.Sin(m.timeAlive*magicMissileWeaveFrequency) * magicMissileWeaveMagnitude
		weaveDX := -dirToTargetY * oscillationFactor // Perpendicular to current direction
		weaveDY := dirToTargetX * oscillationFactor  // Perpendicular to current direction

		// Final velocity components
		vx := (dirToTargetX*m.Speed + weaveDX) / float64(ebiten.TPS())
		vy := (dirToTargetY*m.Speed + weaveDY) / float64(ebiten.TPS())

		m.X += vx
		m.Y += vy

		// Collision with TargetEnemy (and other enemies if piercing)
		distToTargetSq := math.Pow(m.TargetEnemy.X-m.X, 2) + math.Pow(m.TargetEnemy.Y-m.Y, 2)
		// Missile radius is small (e.g. image size / 2), enemy radius is enemyCollisionRadius
		missileRadius := float64(m.Image.Bounds().Dx()) / 2

		// Check collision with any nearby enemy, not just m.TargetEnemy, if it's flying straight after target loss.
		// However, for simplicity with homing, primary collision check is with m.TargetEnemy.
		// If piercing allows hitting others, this loop needs to be broader or missiles need their own small detection radius.
		// For now, stick to collision with m.TargetEnemy for pierce chain initiation.
		// A more general pierce would iterate all enemies here.

		if m.TargetEnemy != nil && m.TargetEnemy.Health > 0 && // Ensure target is still valid for collision check
		   distToTargetSq < math.Pow(missileRadius+m.TargetEnemy.CollisionRadius, 2) {

			if _, alreadyHit := m.hitTargets[m.TargetEnemy]; !alreadyHit {
				m.TargetEnemy.Health -= m.Damage
				m.hitTargets[m.TargetEnemy] = true
				m.PiercesMade++

				if m.PiercesMade > m.MaxPierces {
					m.ToRemove = true
				} else {
					// Missile continues. If TargetEnemy died from this hit, it will fly straight.
					// No explicit re-targeting to a *new* different enemy.
					// It will continue to home on original TargetEnemy if it's still alive.
					// Or fly straight if TargetEnemy died (handled by TargetLoss check at start of loop).
				}
			}
		}
		// If missile has no target (e.g., original target died and it's flying straight)
		// it could collide with other enemies. This requires a broader collision check.
		// For now, we only check collision with the *current* m.TargetEnemy.
		// This means pierce will only effectively work if multiple enemies are super close to the original target OR if the missile path naturally intersects others.
		// A simple extension for "dumb" pierce: iterate all enemies.
		if !m.ToRemove && m.PiercesMade <= m.MaxPierces { // Only if it can still pierce and hasn't been removed
			for _, enemy := range g.enemies {
				if enemy.Health > 0 {
					if _, alreadyHit := m.hitTargets[enemy]; !alreadyHit { // Hasn't hit *this* enemy yet
						distSqToOther := math.Pow(enemy.X-m.X, 2) + math.Pow(enemy.Y-m.Y, 2)
						if distSqToOther < math.Pow(missileRadius+enemy.CollisionRadius, 2) {
							enemy.Health -= m.Damage
							m.hitTargets[enemy] = true
							m.PiercesMade++
							if m.PiercesMade > m.MaxPierces {
								m.ToRemove = true
								break // Stop checking other enemies for this missile
							}
							// Continues, no re-targeting
						}
					}
				}
			}
		}
	}


	// Magic Missile Firing Logic
	if g.player.HasMagicMissile {
		g.player.magicMissileFireTimer -= 1.0 / float64(ebiten.TPS())
		if g.player.magicMissileFireTimer <= 0 {
			g.player.magicMissileFireTimer = magicMissileCooldown

			// Find all valid enemies in range and sort by distance to find N closest
			targetRangeSq := (screenWidth * 1.0) * (screenWidth * 1.0) // Increased range slightly
			type enemyDist struct {
				enemy *Enemy
				distSq float64
			}
			var enemiesInRange []enemyDist

			for _, enemy := range g.enemies {
				if enemy.Health > 0 {
					distSq := math.Pow(enemy.X-g.player.X, 2) + math.Pow(enemy.Y-g.player.Y, 2)
					if distSq <= targetRangeSq {
						enemiesInRange = append(enemiesInRange, enemyDist{enemy, distSq})
					}
				}
			}
			// Sort enemies by distance
			sort.Slice(enemiesInRange, func(i, j int) bool {
				return enemiesInRange[i].distSq < enemiesInRange[j].distSq
			})

			firedMissiles := 0
			for i := 0; i < g.player.MagicMissileCount; i++ {
				var targetForThisMissile *Enemy
				initialAngleOffset := 0.0 // For second missile if no second target

				if i < len(enemiesInRange) {
					targetForThisMissile = enemiesInRange[i].enemy
				} else { // Not enough distinct enemies for this missile index i
					targetForThisMissile = nil // Default to no specific target
					if i == 0 { // First missile, but no enemies were in range at all
						initialAngleOffset = 0 // Default fire right for the first missile
					} else if i == 1 { // Second missile
						if len(enemiesInRange) == 1 { // Only one enemy was found (targeted by missile 0)
							// Fire opposite to the first target's vector
							firstTargetVecX := enemiesInRange[0].enemy.X - g.player.X
							firstTargetVecY := enemiesInRange[0].enemy.Y - g.player.Y
							angleToFirst := math.Atan2(firstTargetVecY, firstTargetVecX)
							initialAngleOffset = angleToFirst + math.Pi
						} else { // No enemies were found at all (len(enemiesInRange) == 0)
							initialAngleOffset = math.Pi // Default fire left for the second missile
						}
					} else {
						// For 3rd+ missiles with no targets, could assign fixed spread angles or break
						break // Or assign a default angle based on 'i'
					}
				}

				missileAngle := initialAngleOffset // Use offset if target is nil
				if targetForThisMissile != nil {
					missileAngle = math.Atan2(targetForThisMissile.Y-g.player.Y, targetForThisMissile.X-g.player.X)
				}

				newMissile := &MagicMissile{
					X:            g.player.X,
					Y:            g.player.Y,
					Speed:        magicMissileSpeed,
					Damage:       magicMissileDamage,
					TargetEnemy:  targetForThisMissile, // Can be nil if firing directionally
					CurrentAngle: missileAngle,       // Initial angle based on target or offset
					timeAlive:    0,
					Image:        magicMissileImage,
					ToRemove:     false,
					MaxPierces:   g.player.MagicMissilePiercing, // Restored
					PiercesMade:  0,                             // Restored
					hitTargets:   make(map[*Enemy]bool),         // Restored
				}
				g.magicMissiles = append(g.magicMissiles, newMissile)
				firedMissiles++
			}
		}
	}
} // This is the correct end of updateGameplayScreen()

func (g *Game) updateLevelUpSelectionScreen() {
	// Use constants for card dimensions for consistency with drawing
	totalLayoutWidth := levelUpCardWidth*2 + levelUpCardSpacing // Total width taken by two cards and spacing

	startX := (float64(screenWidth) - totalLayoutWidth) / 2 // Starting X for the first card
	cardY := (float64(screenHeight) - levelUpCardHeight) / 2 // Y position for both cards (top edge)

	card1Rect := image.Rect(
		int(startX),
		int(cardY),
		int(startX+levelUpCardWidth),
		int(cardY+levelUpCardHeight),
	)
	card2X := startX + levelUpCardWidth + levelUpCardSpacing
	card2Rect := image.Rect(
		int(card2X),
		int(cardY),
		int(card2X+levelUpCardWidth),
		int(cardY+levelUpCardHeight),
	)

	mx, my := ebiten.CursorPosition()
	mousePoint := image.Point{X: mx, Y: my}

	// Keyboard Navigation
	if inpututil.IsKeyJustPressed(ebiten.KeyLeft) || inpututil.IsKeyJustPressed(ebiten.KeyA) {
		g.levelUpSelectedCardIndex--
		if g.levelUpSelectedCardIndex < 0 {
			g.levelUpSelectedCardIndex = 1 // Wrap (assuming 2 options 0 and 1)
		}
		g.levelUpSelectionInputMode = "keyboard"
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyRight) || inpututil.IsKeyJustPressed(ebiten.KeyD) {
		g.levelUpSelectedCardIndex++
		if g.levelUpSelectedCardIndex > 1 { // Assuming 2 options
			g.levelUpSelectedCardIndex = 0 // Wrap
		}
		g.levelUpSelectionInputMode = "keyboard"
	}

	// Mouse Hover for visual selection update (if mode is mouse)
	if g.levelUpSelectionInputMode == "mouse" {
		if mousePoint.In(card1Rect) && g.currentPowerUpChoices[0] != nil {
			g.levelUpSelectedCardIndex = 0
		} else if mousePoint.In(card2Rect) && g.currentPowerUpChoices[1] != nil {
			g.levelUpSelectedCardIndex = 1
		}
		// If mouse is not over any valid card, selection remains.
	}
	// If mode is "keyboard", mouse hover does not change logical selection index.
	// Visual hover effect could still be drawn based on mousePoint directly in Draw,
	// but logical selection for Enter key should stick to keyboard choice.
	// For simplicity, highlight will follow g.levelUpSelectedCardIndex.

	// Confirmation Action
	actionConfirmed := false
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		actionConfirmed = true // Keyboard confirms current g.levelUpSelectedCardIndex
	}

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		if mousePoint.In(card1Rect) && g.currentPowerUpChoices[0] != nil {
			g.levelUpSelectedCardIndex = 0    // Click selects this card
			g.levelUpSelectionInputMode = "mouse" // Click switches mode to mouse
			actionConfirmed = true
		} else if mousePoint.In(card2Rect) && g.currentPowerUpChoices[1] != nil {
			g.levelUpSelectedCardIndex = 1    // Click selects this card
			g.levelUpSelectionInputMode = "mouse" // Click switches mode to mouse
			actionConfirmed = true
		}
	}

	if actionConfirmed {
		// Ensure a valid choice is made if only one power-up was available
		if g.currentPowerUpChoices[g.levelUpSelectedCardIndex] == nil && g.levelUpSelectedCardIndex == 1 && g.currentPowerUpChoices[0] != nil {
			// If second choice is nil and selected, but first is not, default to first.
			// This case might occur if only one powerup was presented.
			g.levelUpSelectedCardIndex = 0
		}


		chosenPowerUp := g.currentPowerUpChoices[g.levelUpSelectedCardIndex]
		if chosenPowerUp != nil {
			log.Printf("Player selected Power-Up: %s (ID: %s)", chosenPowerUp.Title, chosenPowerUp.ID)
			g.player.AcquiredPowerUps = append(g.player.AcquiredPowerUps, chosenPowerUp.ID)

			// Apply power-up effects
			switch chosenPowerUp.ID {
			case "PUP001": // Speed Boost
				g.player.Speed += 20 // Flat increase
			case "PUP002": // Attack Speed Up
				g.player.AttackCooldown *= 0.85 // 15% faster
				if g.player.AttackCooldown < 0.1 { // Prevent excessively fast attacks
					g.player.AttackCooldown = 0.1
				}
			case "PUP003": // Increased XP Gain
				g.player.XPMultiplier += 0.25
			case "PUP004": // Larger Attack Radius
				g.player.AttackMaxRadius *= 1.25
			case "PUP005": // Extra Health
				bonusHealth := 25
				g.player.MaxHealth += bonusHealth
				g.player.CurrentHealth += bonusHealth // Heal by the bonus amount as well
				if g.player.CurrentHealth > g.player.MaxHealth {
					g.player.CurrentHealth = g.player.MaxHealth // Clamp to new max
				}
			case "PUP006": // Magic Missile (Base)
				g.player.HasMagicMissile = true
				if g.player.MagicMissileCount < 1 { // Only set to 1 if they don't have it or somehow count is 0
					g.player.MagicMissileCount = 1
				}
				// If they already have it, this PUP appearing again is a bug in selection logic,
				// or it could be a +1 missile count if we change PUP007.
				// For now, ensures they have at least 1 missile.
				g.player.magicMissileFireTimer = magicMissileCooldown
			case "PUP007": // Twin Barrage (Additional Missile)
				if g.player.HasMagicMissile { // Prerequisite: must have missiles first
					g.player.MagicMissileCount++ // For now, allow stacking, e.g. 3, 4 missiles
					// Consider adding a g.player.MaxMagicMissileCount if desired
				} else {
					// If they get this without PUP006, grant PUP006 effects too
					g.player.HasMagicMissile = true
					g.player.MagicMissileCount = 2 // Gets base + this one
					g.player.magicMissileFireTimer = magicMissileCooldown
				}
			case "PUP008": // Phantom Edge (Piercing) - Restored
				g.player.MagicMissilePiercing++
			}
		}
		g.currentState = StateGameplay // Resume gameplay
	}
}

// prepareLevelUpChoices selects two distinct power-ups to offer the player.
func (g *Game) prepareLevelUpChoices() {
	if len(availablePowerUps) < 2 {
		// Handle cases where not enough unique power-ups are available
		// For now, this basic version might offer duplicates or nil if fewer than 2.
		// A more robust version would filter out already acquired one-time power-ups
		// or ensure variety.
		if len(availablePowerUps) == 1 {
			g.currentPowerUpChoices[0] = &availablePowerUps[0]
			g.currentPowerUpChoices[1] = nil // Or a generic choice like "Minor XP Boost"
		} else if len(availablePowerUps) == 0 {
			g.currentPowerUpChoices[0] = nil
			g.currentPowerUpChoices[1] = nil
		}
		g.levelUpSelectedCardIndex = 0
		return
	}

	// Shuffle availablePowerUps to get random choices
	// Create a copy to shuffle if you don't want to alter the original slice order permanently
	shuffledChoices := make([]PowerUpDefinition, len(availablePowerUps))
	copy(shuffledChoices, availablePowerUps)

	rand.Shuffle(len(shuffledChoices), func(i, j int) {
		shuffledChoices[i], shuffledChoices[j] = shuffledChoices[j], shuffledChoices[i]
	})

	g.currentPowerUpChoices[0] = &shuffledChoices[0]
	g.currentPowerUpChoices[1] = &shuffledChoices[1]

	g.levelUpSelectedCardIndex = 0    // Default to selecting the first card
	g.levelUpSelectionInputMode = "keyboard" // Default to keyboard mode when screen appears
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
	case StateLevelUpSelection:
		g.drawLevelUpSelectionScreen(screen)
	case StateLoseScreen:
		g.drawLoseScreen(screen)
	}

	// Draw Dev Interface if active (on top of everything else)
	if g.showDevInterface {
		g.drawDevInterface(screen)
	}
}

func (g *Game) drawDevInterface(screen *ebiten.Image) {
	devInterfaceX := 10.0
	devInterfaceY := float64(screenHeight) - 250.0 // Position from bottom
	devInterfaceWidth := 280.0
	devInterfaceHeight := 240.0
	padding := 5.0
	lineHeight := 15.0

	// Background panel
	bgColor := color.NRGBA{R: 0, G: 0, B: 0, A: 180}
	ebitenutil.DrawRect(screen, devInterfaceX, devInterfaceY, devInterfaceWidth, devInterfaceHeight, bgColor)

	currentY := devInterfaceY + padding

	// Helper to draw next line
	drawLine := func(text string) {
		ebitenutil.DebugPrintAt(screen, text, int(devInterfaceX+padding), int(currentY))
		currentY += lineHeight
	}

	drawLine(fmt.Sprintf("FPS: %.2f, TPS: %.2f", ebiten.ActualFPS(), ebiten.ActualTPS()))

	var stateStr string
	switch g.currentState {
	case StateTitleScreen: stateStr = "TitleScreen"
	case StateGameplay: stateStr = "Gameplay"
	case StateLevelUpSelection: stateStr = "LevelUpSelection"
	case StateLoseScreen: stateStr = "LoseScreen"
	default: stateStr = "Unknown"
	}
	drawLine(fmt.Sprintf("State: %s", stateStr))

	drawLine("--- Player ---")
	drawLine(fmt.Sprintf("Pos: (%.1f, %.1f)", g.player.X, g.player.Y))
	drawLine(fmt.Sprintf("HP: %d/%d, Lvl: %d", g.player.CurrentHealth, g.player.MaxHealth, g.player.Level))
	drawLine(fmt.Sprintf("XP: %d/%d", g.player.CurrentXP, g.player.XPToNextLevel))
	drawLine(fmt.Sprintf("Speed: %.1f, AtkCD: %.2f", g.player.Speed, g.player.AttackCooldown))
	drawLine(fmt.Sprintf("AtkRadius: %.1f, XPMulti: %.2f", g.player.AttackMaxRadius, g.player.XPMultiplier))
	drawLine(fmt.Sprintf("InvulTime: %.2f", g.player.invulnerabilityTimer))
	if len(g.player.AcquiredPowerUps) > 0 {
		drawLine(fmt.Sprintf("PowerUps: %s", strings.Join(g.player.AcquiredPowerUps, ", ")))
	} else {
		drawLine("PowerUps: None")
	}


	drawLine("--- Game ---")
	drawLine(fmt.Sprintf("Enemies: %d, Attacks: %d, Orbs: %d", len(g.enemies), len(g.pulseAttacks), len(g.xpOrbs)))
	drawLine(fmt.Sprintf("Cam: (%.1f, %.1f)", g.camX, g.camY))
}


func (g *Game) drawLevelUpSelectionScreen(screen *ebiten.Image) {
	// 1. Draw paused gameplay screen as background
	g.drawGameplayScreen(screen)

	// 2. Draw a semi-transparent overlay to dim the background
	overlayColor := color.NRGBA{R: 0, G: 0, B: 0, A: 180} // Dark semi-transparent
	ebitenutil.DrawRect(screen, 0, 0, float64(screenWidth), float64(screenHeight), overlayColor)

	// 3. Define card positions and dimensions (consistent with updateLevelUpSelectionScreen)
	totalLayoutWidth := levelUpCardWidth*2 + levelUpCardSpacing
	startX := (float64(screenWidth) - totalLayoutWidth) / 2
	cardY := (float64(screenHeight) - levelUpCardHeight) / 2

	cardPositions := [2][2]float64{
		{startX, cardY},
		{startX + levelUpCardWidth + levelUpCardSpacing, cardY},
	}

	// 4. Draw the two power-up cards
	for i, choice := range g.currentPowerUpChoices {
		if choice == nil {
			// Optionally draw an empty card slot or skip
			// For now, draw a slightly different background if choice is nil
			cardPosX := cardPositions[i][0]
			cardPosY := cardPositions[i][1]
			emptyCardBgColor := color.NRGBA{R: 30, G: 30, B: 40, A: 200}
			ebitenutil.DrawRect(screen, cardPosX, cardPosY, levelUpCardWidth, levelUpCardHeight, emptyCardBgColor)
			ebitenutil.DebugPrintAt(screen, "[No Option]", int(cardPosX+levelUpCardPadding), int(cardPosY+levelUpCardHeight/2))
			continue
		}

		cardPosX := cardPositions[i][0]
		cardPosY := cardPositions[i][1]

		cardBgColor := color.NRGBA{R: 40, G: 40, B: 60, A: 230} // Card background
		if i == g.levelUpSelectedCardIndex {
			cardBgColor = color.NRGBA{R: 60, G: 60, B: 90, A: 255} // Selected card brighter
		}
		ebitenutil.DrawRect(screen, cardPosX, cardPosY, levelUpCardWidth, levelUpCardHeight, cardBgColor)

		// Card border for selected
		if i == g.levelUpSelectedCardIndex {
			borderColor := color.NRGBA{R: 180, G: 180, B: 255, A: 255} // Light highlight border
			ebitenutil.DrawRect(screen, cardPosX-2, cardPosY-2, levelUpCardWidth+4, 2, borderColor) // Top
			ebitenutil.DrawRect(screen, cardPosX-2, cardPosY+levelUpCardHeight, levelUpCardWidth+4, 2, borderColor) // Bottom
			ebitenutil.DrawRect(screen, cardPosX-2, cardPosY, 2, levelUpCardHeight, borderColor) // Left
			ebitenutil.DrawRect(screen, cardPosX+levelUpCardWidth, cardPosY, 2, levelUpCardHeight, borderColor) // Right
		}

		titleX := int(cardPosX + levelUpCardPadding)
		titleY := int(cardPosY + levelUpTitleOffsetY)
		ebitenutil.DebugPrintAt(screen, choice.Title, titleX, titleY)

		descY := int(cardPosY + levelUpDescOffsetY)
		wrappedDesc := g.wrapText(choice.Description, int(levelUpCardWidth-2*levelUpCardPadding)/6) // Approx chars per line
		for j, line := range wrappedDesc {
			ebitenutil.DebugPrintAt(screen, line, titleX, descY+(j*15))
		}
	}

	// 5. Draw prompt text
	promptText := "Choose an Upgrade! (Arrows/Mouse, Enter/Click)"
	promptTextPixelWidth := len(promptText) * 6
	promptX := (screenWidth - promptTextPixelWidth) / 2
	promptY := int(cardY + levelUpCardHeight + levelUpCardSpacing + 10)
	if promptY > screenHeight - 30 { // Ensure it's on screen
		promptY = screenHeight - 30
	}
	ebitenutil.DebugPrintAt(screen, promptText, promptX, promptY)
}


// wrapText is a helper function to break a long string into lines of roughly maxLength characters.
func (g *Game) wrapText(text string, maxCharsPerLine int) []string {
	var lines []string
	var currentLine string
	words := strings.Fields(text)

	if len(words) == 0 {
		return []string{text} // Return original text if no words (e.g. empty or all spaces)
	}

	for _, word := range words {
		if len(currentLine) == 0 {
			currentLine = word
		} else if len(currentLine)+len(word)+1 <= maxCharsPerLine {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}
	if len(currentLine) > 0 {
		lines = append(lines, currentLine)
	}
	return lines
}


func (g *Game) drawTitleScreen(screen *ebiten.Image) {
	screen.Fill(color.NRGBA{R: 20, G: 20, B: 40, A: 255}) // Dark blue background for title

	titleText := "Go Survivor"
	titleTextPixelWidth := len(titleText) * 6 // Approximate pixel width of default font characters
	titleX := (screenWidth - titleTextPixelWidth) / 2
	ebitenutil.DebugPrintAt(screen, titleText, titleX, screenHeight/4)

	optionStartY := screenHeight / 2
	lineHeight := 20 // Spacing between menu items

	// Option 1: Start Game
	startText := "Start Game"
	if g.titleSelectedOption == 0 {
		startText = "> " + startText
	}
	startTextPixelWidth := len(startText) * 6
	startX := (screenWidth - startTextPixelWidth) / 2
	ebitenutil.DebugPrintAt(screen, startText, startX, optionStartY)

	// Option 2: Quit
	quitText := "Quit"
	if g.titleSelectedOption == 1 {
		quitText = "> " + quitText
	}
	quitTextPixelWidth := len(quitText) * 6
	quitX := (screenWidth - quitTextPixelWidth) / 2
	ebitenutil.DebugPrintAt(screen, quitText, quitX, optionStartY+lineHeight)

	instructions := "Use Up/Down Arrows & Enter"
	instructionsPixelWidth := len(instructions) * 6
	instructionsX := (screenWidth - instructionsPixelWidth) / 2
	ebitenutil.DebugPrintAt(screen, instructions, instructionsX, screenHeight-50)
}

func (g *Game) drawGameplayScreen(screen *ebiten.Image) {
	// This will contain the existing core game draw logic
	// Optional: Fill background
	// screen.Fill(color.NRGBA{R: 10, G: 10, B: 30, A: 255})

	// Draw Obstacles
	for _, obs := range g.obstacles {
		if obs.Image != nil {
			opts := &ebiten.DrawImageOptions{}

			// Get original image dimensions for scaling
			imgWidth := float64(obs.Image.Bounds().Dx())
			imgHeight := float64(obs.Image.Bounds().Dy())

			if imgWidth > 0 && imgHeight > 0 { // Avoid division by zero if image is empty
				scaleX := obs.Width / imgWidth
				scaleY := obs.Height / imgHeight
				opts.GeoM.Scale(scaleX, scaleY)
			}

			// Obstacle X,Y is top-left for its (now potentially scaled) image.
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

	// Draw Magic Missiles (relative to camera)
	for _, m := range g.magicMissiles {
		if !m.ToRemove && m.Image != nil {
			opts := &ebiten.DrawImageOptions{}
			// Center the image before rotation and translation
			opts.GeoM.Translate(-float64(m.Image.Bounds().Dx())/2, -float64(m.Image.Bounds().Dy())/2)
			// Rotate
			opts.GeoM.Rotate(m.CurrentAngle)
			// Translate to missile's world position
			opts.GeoM.Translate(m.X, m.Y)
			// Apply camera view
			opts.GeoM.Translate(-g.camX, -g.camY)
			screen.DrawImage(m.Image, opts)
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
	// The old "LEVEL UP!" message display is removed as level up now transitions to a new state.
}

func (g *Game) drawLoseScreen(screen *ebiten.Image) {
	screen.Fill(color.NRGBA{R: 50, G: 20, B: 20, A: 255}) // Dark red background for lose screen

	loseMsgText := "YOU DIED"
	loseMsgTextPixelWidth := len(loseMsgText) * 6
	loseMsgX := (screenWidth - loseMsgTextPixelWidth) / 2
	ebitenutil.DebugPrintAt(screen, loseMsgText, loseMsgX, screenHeight/4)

	finalScoreText := "Final Score: " + strconv.Itoa(g.score)
	finalScorePixelWidth := len(finalScoreText) * 6
	finalScoreX := (screenWidth - finalScorePixelWidth) / 2
	ebitenutil.DebugPrintAt(screen, finalScoreText, finalScoreX, screenHeight/4+40)

	finalLevelText := "Level Reached: " + strconv.Itoa(g.player.Level)
	finalLevelPixelWidth := len(finalLevelText) * 6
	finalLevelX := (screenWidth - finalLevelPixelWidth) / 2
	ebitenutil.DebugPrintAt(screen, finalLevelText, finalLevelX, screenHeight/4+60)

	optionStartY := screenHeight/2 + 30
	lineHeight := 20

	retryText := "Retry"
	if g.loseSelectedOption == 0 {
		retryText = "> " + retryText
	}
	retryTextPixelWidth := len(retryText) * 6
	retryX := (screenWidth - retryTextPixelWidth) / 2
	ebitenutil.DebugPrintAt(screen, retryText, retryX, optionStartY)

	menuText := "Main Menu"
	if g.loseSelectedOption == 1 {
		menuText = "> " + menuText
	}
	menuTextPixelWidth := len(menuText) * 6
	menuX := (screenWidth - menuTextPixelWidth) / 2
	ebitenutil.DebugPrintAt(screen, menuText, menuX, optionStartY+lineHeight)

	instructions := "Use Up/Down Arrows & Enter"
	instructionsPixelWidth := len(instructions) * 6
	instructionsX := (screenWidth - instructionsPixelWidth) / 2
	ebitenutil.DebugPrintAt(screen, instructions, instructionsX, screenHeight-50)
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
