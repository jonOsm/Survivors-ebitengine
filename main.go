package main

import (
	"image/color"
	"log"
	"math"
	"math/rand"
	"strconv"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

const (
	screenWidth  = 800
	screenHeight = 600
)

var (
	playerImage       *ebiten.Image
	enemyImage        *ebiten.Image
	pulseAttackImage  *ebiten.Image // Placeholder, will be a transparent square
)

// Game implements ebiten.Game interface.
type Game struct {
	player         *Player
	enemies        []*Enemy
	pulseAttacks   []*PulseAttack
	attackTimer    float64 // Seconds
	spawnTimer     float64 // Seconds
	score          int
	gameOver       bool
	enemySpawnRate float64 // Seconds between enemy spawns
	enemiesPerWave int
}

// Player represents the player character.
type Player struct {
	X, Y  float64
	Speed float64
	Image *ebiten.Image
	CollisionRadius float64
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
	enemySpeed            = 100 // Pixels per second
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
	pulseAttackMaxRadius      = 150
	pulseAttackExpansionSpeed = 100 // pixels per second
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

	rand.Seed(time.Now().UnixNano())
}

// NewGame initializes a new game.
func NewGame() *Game {
	g := &Game{
		player: &Player{
			X:               screenWidth / 2,
			Y:               screenHeight / 2,
			Speed:           200, // Pixels per second
			Image:           playerImage,
			CollisionRadius: playerCollisionRadius,
		},
		enemies:        []*Enemy{},
		pulseAttacks:   []*PulseAttack{},
		attackTimer:    0,
		spawnTimer:     0,
		score:          0,
		gameOver:       false,
		enemySpawnRate: 3.0, // Initial spawn rate: 3 seconds
		enemiesPerWave: 5,   // Initial enemies per wave
	}
	return g
}

// Update proceeds the game state.
// Update is called every tick (1/60 [s] by default).
func (g *Game) Update() error {
	if g.gameOver {
		return nil // Stop updates if game is over
	}

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

	g.player.X += dx * g.player.Speed / float64(ebiten.TPS())
	g.player.Y += dy * g.player.Speed / float64(ebiten.TPS())

	// Keep player within screen bounds
	playerWidth := float64(g.player.Image.Bounds().Dx())
	playerHeight := float64(g.player.Image.Bounds().Dy())
	g.player.X = clamp(g.player.X, playerWidth/2, screenWidth-playerWidth/2)
	g.player.Y = clamp(g.player.Y, playerHeight/2, screenHeight-playerHeight/2)

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
			var ex, ey float64
			side := rand.Intn(4) // 0: top, 1: bottom, 2: left, 3: right
			const spawnMargin = 50 // Defines how far off-screen enemies will spawn.

			switch side {
			case 0: // Top
				ex = rand.Float64() * screenWidth
				ey = -spawnMargin
			case 1: // Bottom
				ex = rand.Float64() * screenWidth
				ey = screenHeight + spawnMargin
			case 2: // Left
				ex = -spawnMargin
				ey = rand.Float64() * screenHeight
			case 3: // Right
				ex = screenWidth + spawnMargin
				ey = rand.Float64() * screenHeight
			}

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
		enemy.X += normalizedDx * enemy.Speed / float64(ebiten.TPS())
		enemy.Y += normalizedDy * enemy.Speed / float64(ebiten.TPS())
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
			// Collision occurs if the distance between player and enemy centers
			// is less than the sum of their collision radii.
			if dist < g.player.CollisionRadius+enemy.CollisionRadius {
				g.gameOver = true // Set game over state.
				// The main update loop will detect g.gameOver and stop further game logic updates.
				break // No need to check other enemies if game is already over.
			}
		}
	}

	// Entity Cleanup: Remove "dead" entities (if game is not over).
	// This is done by creating new slices containing only the "alive" entities
	// and then replacing the old slices. This is an efficient way to filter slices in Go.
	if !g.gameOver {
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
	}

	return nil
}

// Draw draws the game screen.
// Draw is called every frame (typically 1/60[s] for 60Hz display).
func (g *Game) Draw(screen *ebiten.Image) {
	// Draw Player
	if g.player != nil && g.player.Image != nil {
		playerOpts := &ebiten.DrawImageOptions{}
		playerOpts.GeoM.Translate(g.player.X-float64(g.player.Image.Bounds().Dx())/2, g.player.Y-float64(g.player.Image.Bounds().Dy())/2)
		screen.DrawImage(g.player.Image, playerOpts)
	}

	// Draw Pulse Attacks
	for _, attack := range g.pulseAttacks {
		if attack.Radius > 0 && attack.Image != nil {
			opts := &ebiten.DrawImageOptions{}
			scale := attack.Radius * 2
			opts.GeoM.Scale(scale, scale)
			opts.GeoM.Translate(attack.X-attack.Radius, attack.Y-attack.Radius)
			screen.DrawImage(attack.Image, opts)
		}
	}

	// Draw Enemies
	for _, enemy := range g.enemies {
		if enemy.Image != nil {
			opts := &ebiten.DrawImageOptions{}
			opts.GeoM.Translate(enemy.X-float64(enemy.Image.Bounds().Dx())/2, enemy.Y-float64(enemy.Image.Bounds().Dy())/2)
			screen.DrawImage(enemy.Image, opts)
		}
	}

	// Display Score and Game Over Message
	if g.gameOver {
		ebitenutil.DebugPrint(screen, "GAME OVER\nFinal Score: "+strconv.Itoa(g.score))
	} else {
		ebitenutil.DebugPrint(screen, "Score: "+strconv.Itoa(g.score))
	}
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
