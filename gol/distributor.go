package gol

import (
	"fmt"
	"sync"
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
	world := make([][]uint8, p.ImageHeight)
	for i := range world {
		world[i] = make([]uint8, p.ImageWidth)
	}
	for i := 0; i < p.ImageHeight; i++ {
		for k := 0; k < p.ImageWidth; k++ {
			world[i][k] = <-c.ioInput
		}
	}

	turn := p.Turns
	if turn != 0 {
		//new world is to store next state----------------------------------------------------------------------------------
		newWorld := make([][]uint8, p.ImageHeight)
		for i := range newWorld {
			newWorld[i] = make([]uint8, p.ImageWidth)
		}

		//ticker send signal every 2 second to report alive cell
		ticker := time.NewTicker(2 * time.Second)

		copyWhole(newWorld, world)
		if p.Threads == 1 {
			// Single-threaded execution
			for t := 0; t < turn; t++ {
				newWorld = calculateNextState(0, p.ImageHeight, world)
				copyWhole(world, newWorld)
				c.events <- TurnComplete{CompletedTurns: 1}
			}
		} else {
			var mutex sync.Mutex //a lock to protect livecell and completed without race condition
			liveCell := 0
			completed := 0
			//go routine using select and infinite loop to report
			go func() {
				for {
					select {
					case <-ticker.C:
						mutex.Lock()
						c.events <- AliveCellsCount{CellsCount: liveCell, CompletedTurns: completed}
						mutex.Unlock()
					}
				}
			}()

			portionHeight := p.ImageHeight / p.Threads
			for t := 0; t < turn; t++ {
				mutex.Lock()
				liveCell = len(calculateAliveCells(world))
				mutex.Unlock()

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
				mutex.Lock()
				completed++
				mutex.Unlock()
				c.events <- TurnComplete{CompletedTurns: t + 1}
			}
		}
		defer ticker.Stop()
	} else {
		c.events <- AliveCellsCount{CellsCount: len(calculateAliveCells(world)), CompletedTurns: 0}
		c.events <- TurnComplete{CompletedTurns: 0}

	}
	// TODO: Report the final state using FinalTurnCompleteEvent.
	c.ioCommand <- ioCommand(0)
	fileName = fmt.Sprintf("%dx%dx%d", p.ImageHeight, p.ImageWidth, turn)
	c.ioFilename <- fileName
	for i := 0; i < p.ImageHeight; i++ {
		for k := 0; k < p.ImageWidth; k++ {
			c.ioOutput <- world[i][k]
		}
	}
	alive := calculateAliveCells(world)
	c.events <- FinalTurnComplete{CompletedTurns: turn, Alive: alive}

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	c.events <- StateChange{turn, Quitting}
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

func calculateNearAlive(world [][]uint8, row, col int) int {
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

func copyWhole(dst, src [][]uint8) {
	for i := range src {
		copy(dst[i], src[i])
	}
}
func calculateNextState(startY, endY int, world [][]uint8) [][]uint8 {

	newWorld := make([][]uint8, len(world))
	for i := range newWorld {
		newWorld[i] = make([]uint8, len(world[i]))
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
func calculateAliveCells(world [][]uint8) []util.Cell {
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
