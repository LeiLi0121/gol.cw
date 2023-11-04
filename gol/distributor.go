package gol

import (
	"fmt"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	// TODO: Create a 2D slice to store the world.
	fileName := fmt.Sprintf("%dx%d", p.ImageHeight, p.ImageWidth)
	c.ioCommand <- ioCommand(1)
	c.ioFilename <- fileName
	//world recieve the image from io-----------------------------------------------------------------------------------
	world := make([][]byte, p.ImageHeight)
	for i := range world {
		world[i] = make([]byte, p.ImageWidth)
	}
	for i := 0; i < p.ImageHeight; i++ {
		for k := 0; k < p.ImageWidth; k++ {
			world[i][k] = <-c.ioInput
		}
	}
	//new world is to store next state----------------------------------------------------------------------------------

	turn := p.Turns
	if turn != 0 {
		completedTurn := 0
		ticker := time.NewTicker(2 * time.Second)
		go func() {
			for {
				select {
				case <-ticker.C:
					c.events <- AliveCellsCount{CompletedTurns: completedTurn, CellsCount: len(calculateAliveCells(world))}
				}
			}
		}()
		newWorld := make([][]byte, p.ImageHeight)
		for i := range newWorld {
			newWorld[i] = make([]byte, p.ImageWidth)
		}
		copyWhole(newWorld, world)
		if p.Threads == 1 {
			// Single-threaded execution
			for t := 0; t < turn; t++ {
				newWorld = calculateNextState(0, p.ImageHeight, world)
				copyWhole(world, newWorld)
				completedTurn++
			}
		} else {
			portionHeight := p.ImageHeight / p.Threads
			for t := 0; t < turn; t++ {
				parts := make([]chan [][]uint8, p.Threads)
				for i := range parts {
					parts[i] = make(chan [][]uint8, 1)
				}
				for i := 0; i < p.Threads; i++ {
					go func(i int) {
						if i == p.Threads-1 {
							startY := i * portionHeight
							endY := p.ImageHeight
							parts[i] <- calculateNextState(startY, endY, world)
						} else {
							startY := i * portionHeight
							endY := (i + 1) * portionHeight
							parts[i] <- calculateNextState(startY, endY, world)
						}
					}(i)

				}
				for i := 0; i < p.Threads; i++ {
					part := <-parts[i]
					if i != p.Threads-1 {
						for h := 0; h < portionHeight; h++ {
							copy(newWorld[i*portionHeight+h], part[i*portionHeight+h])
						}
					} else {
						for h := 0; h < p.ImageHeight-i*portionHeight; h++ {
							copy(newWorld[i*portionHeight+h], part[i*portionHeight+h])
						}
					}

				}
				copyWhole(world, newWorld)
				completedTurn++
				c.events <- TurnComplete{CompletedTurns: t}
			}

		}
		defer ticker.Stop()
	}

	// TODO: Report the final state using FinalTurnCompleteEvent.
	alive := calculateAliveCells(world)
	c.events <- FinalTurnComplete{CompletedTurns: turn, Alive: alive}
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

func calculateNearAlive(world [][]byte, row, col int) int {
	adjacent := []Pair{
		{-1, -1}, {-1, 0}, {-1, 1},
		{0, -1}, {0, 1},
		{1, -1}, {1, 0}, {1, 1},
	}
	//add to get all adj nodes within 0-15
	for i := 0; i < 8; i++ {
		adjacent[i].y += row
		if adjacent[i].y == len(world) {
			adjacent[i].y = 0
		}
		if adjacent[i].y == -1 {
			adjacent[i].y = len(world) - 1
		}
		adjacent[i].x += col
		if adjacent[i].x == len(world[i]) {
			adjacent[i].x = 0
		}
		if adjacent[i].x == -1 {
			adjacent[i].x = len(world[i]) - 1
		}
	}
	//count alive using for
	count := 0
	for _, node := range adjacent {
		if world[node.y][node.x] == 255 {
			count++
		}
	}
	return count
}

func copyWhole(dst, src [][]byte) {
	for i := range src {
		copy(dst[i], src[i])
	}
}
func calculateNextState(startY, endY int, world [][]byte) [][]byte {

	newWorld := make([][]byte, len(world))
	for i := range newWorld {
		newWorld[i] = make([]byte, len(world[i]))
	}
	for i := startY; i < endY; i++ { //each row
		for k := 0; k < len(world[i]); k++ { //each item in row
			numOfAlive := calculateNearAlive(world, i, k)
			currentNode := world[i][k]

			//rules for updating the cell state
			if world[i][k] == 255 {
				if numOfAlive < 2 {
					newWorld[i][k] = 0
				} else if numOfAlive == 2 || numOfAlive == 3 {
					newWorld[i][k] = currentNode
				} else if numOfAlive > 3 {
					newWorld[i][k] = 0
				}
			} else if currentNode == 0 && numOfAlive == 3 {
				newWorld[i][k] = 255
			}
		}
	}

	return newWorld
}
func calculateAliveCells(world [][]byte) []util.Cell {
	var alive []util.Cell
	for i := 0; i < len(world); i++ {
		for k := 0; k < len(world[i]); k++ {
			if world[i][k] == 255 {
				alive = append(alive, util.Cell{X: k, Y: i})
			}
		}
	}
	return alive
}

type Pair struct {
	y int
	x int
}
