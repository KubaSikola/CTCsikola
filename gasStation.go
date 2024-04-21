package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"
)

// Config represents the configuration of the simulation
type Car struct {
	CarType        string
	TotalTimeSpent time.Duration
	QueueStartTime time.Time
}

type Config struct {
	Cars struct {
		Count          int `json:"count"`
		ArrivalTimeMin int `json:"arrival_time_min"`
		ArrivalTimeMax int `json:"arrival_time_max"`
	} `json:"cars"`
	Stations struct {
		Gas struct {
			Count        int `json:"count"`
			ServeTimeMin int `json:"serve_time_min"`
			ServeTimeMax int `json:"serve_time_max"`
		} `json:"gas"`
		Diesel struct {
			Count        int `json:"count"`
			ServeTimeMin int `json:"serve_time_min"`
			ServeTimeMax int `json:"serve_time_max"`
		} `json:"diesel"`
		Lpg struct {
			Count        int `json:"count"`
			ServeTimeMin int `json:"serve_time_min"`
			ServeTimeMax int `json:"serve_time_max"`
		} `json:"lpg"`
		Electric struct {
			Count        int `json:"count"`
			ServeTimeMin int `json:"serve_time_min"`
			ServeTimeMax int `json:"serve_time_max"`
		} `json:"electric"`
	} `json:"stations"`
	Registers struct {
		Count         int `json:"count"`
		HandleTimeMin int `json:"handle_time_min"`
		HandleTimeMax int `json:"handle_time_max"`
	} `json:"registers"`
}

var config Config
var err error

func main() {
	config, err = readConfigFromFile("config.json")
	if err != nil {
		log.Fatal("Error reading config file:", err)
	}

	carTypes := map[string]int{
		"gas":      0,
		"diesel":   1,
		"lpg":      2,
		"electric": 3,
	}

	numOfStations := map[string]int{
		"gas":      config.Stations.Gas.Count,
		"diesel":   config.Stations.Diesel.Count,
		"lpg":      config.Stations.Lpg.Count,
		"electric": config.Stations.Electric.Count,
	}

	nRegisters := config.Registers.Count

	const nTypes = 4
	const maxQueueSize = 100

	var stationsWg sync.WaitGroup
	var carArrivalWg sync.WaitGroup
	var registersWg sync.WaitGroup

	registersQueue := make(chan Car, maxQueueSize)
	stationQueues := make([]chan Car, nTypes)

	totalTimeInQueue := make(map[string]time.Duration)
	maxTimeInQueue := make(map[string]time.Duration)
	var maxTimeInRegisterQueue time.Duration
	var totalTimeRegisterQueue time.Duration
	nCarTypes := make(map[string]int)

	for i := 0; i < nTypes; i++ {
		stationQueues[i] = make(chan Car, maxQueueSize)
	}

	carArrivalWg.Add(1)
	go carsArival(stationQueues, carTypes, &carArrivalWg, &nCarTypes)

	for carType, nStations := range numOfStations {
		queueIndex := carTypes[carType]
		for i := 0; i < nStations; i++ {
			stationsWg.Add(1)
			go stationProcess(stationQueues[queueIndex], registersQueue, &stationsWg, carType, &totalTimeInQueue, &maxTimeInQueue)
		}
	}

	for i := 0; i < nRegisters; i++ {
		registersWg.Add(1)
		go registers(registersQueue, &registersWg, &totalTimeRegisterQueue, &maxTimeInRegisterQueue)
	}

	carArrivalWg.Wait()
	for _, queue := range stationQueues {
		close(queue)
	}
	stationsWg.Wait()
	close(registersQueue)
	registersWg.Wait()

	for stationType, totalTime := range totalTimeInQueue {
		if nCarTypes[stationType] != 0 {
			averageTime := time.Duration(int64(totalTime) / int64(nCarTypes[stationType]))
			fmt.Printf("Type: %s\n", stationType)
			fmt.Printf("	Total cars: %d\n", nCarTypes[stationType])
			fmt.Printf("	Total time in queue: %dms\n", totalTime.Milliseconds())
			fmt.Printf("	Average wait at station: %dms\n", averageTime.Milliseconds())
			fmt.Printf("	Max wait time: %dms\n\n", maxTimeInQueue[stationType].Milliseconds())
		}
	}

	averageTimeRegisters := time.Duration(int64(totalTimeRegisterQueue) / int64(config.Cars.Count))
	fmt.Printf("Registers: \n")
	fmt.Printf("	Total time in queue: %dms\n", totalTimeRegisterQueue.Milliseconds())
	fmt.Printf("	Average wait at registers: %dms\n", averageTimeRegisters.Milliseconds())
	fmt.Printf("	Max wait time: %dms", maxTimeInRegisterQueue.Milliseconds())
}

func registers(queue <-chan Car, wg *sync.WaitGroup, totalTimeRegisterQueue *time.Duration, maxTimeRegisterQueue *time.Duration) {
	defer wg.Done()
	maxDuration := config.Registers.HandleTimeMax
	minDuration := config.Registers.HandleTimeMin
	for car := range queue {
		interval, err := getInterval(minDuration, maxDuration)
		if err != nil {
			return
		}
		car.TotalTimeSpent = interval
		timeTaken := time.Since(car.QueueStartTime)
		if timeTaken > *maxTimeRegisterQueue {
			*maxTimeRegisterQueue = timeTaken
		}
		*totalTimeRegisterQueue += timeTaken
		time.Sleep(interval)
	}
}

func stationProcess(stationQueue chan Car, registersQueue chan<- Car, wg *sync.WaitGroup, stationType string, totalTimeInQueue *map[string]time.Duration, maxTimeInQueue *map[string]time.Duration) {
	defer wg.Done()
	var minDuration int
	var maxDuration int
	switch stationType {
	case "diesel":
		maxDuration = config.Stations.Diesel.ServeTimeMax
		minDuration = config.Stations.Diesel.ServeTimeMin
	case "gas":
		maxDuration = config.Stations.Gas.ServeTimeMax
		minDuration = config.Stations.Gas.ServeTimeMin
	case "electric":
		maxDuration = config.Stations.Electric.ServeTimeMax
		minDuration = config.Stations.Electric.ServeTimeMin
	case "lpg":
		maxDuration = config.Stations.Lpg.ServeTimeMax
		minDuration = config.Stations.Lpg.ServeTimeMin
	}
	for car := range stationQueue {
		interval, err := getInterval(minDuration, maxDuration)
		if err != nil {
			return
		}
		car.TotalTimeSpent = interval
		timeInQueue := time.Since(car.QueueStartTime)
		if timeInQueue > (*maxTimeInQueue)[stationType] {
			(*maxTimeInQueue)[stationType] = timeInQueue
		}
		(*totalTimeInQueue)[stationType] += timeInQueue
		time.Sleep(time.Duration(interval.Microseconds()))
		car.QueueStartTime = time.Now()
		registersQueue <- car
	}

}

func carsArival(stationQueues []chan Car, carTypes map[string]int, wg *sync.WaitGroup, nCarTypes *map[string]int) {
	defer wg.Done()
	var n int = config.Cars.Count
	var typesStr []string
	for key := range carTypes {
		typesStr = append(typesStr, key)
	}

	maxTime := config.Cars.ArrivalTimeMax
	minTime := config.Cars.ArrivalTimeMin
	for i := 0; i < n; i++ {

		interval, err := getInterval(minTime, maxTime)
		if err != nil {
			return
		}
		carTypeIndex := rand.Intn(4)
		car := Car{
			CarType:        typesStr[carTypeIndex],
			TotalTimeSpent: 0,
			QueueStartTime: time.Now(),
		}
		(*nCarTypes)[car.CarType] += 1
		queueIndex := carTypes[car.CarType]

		stationQueues[queueIndex] <- car
		if i%100 == 0 {
			println("Cars arived: " + strconv.Itoa(i))
		}
		time.Sleep(interval)
	}

}
func readConfigFromFile(filename string) (Config, error) {
	var config Config
	data, err := os.ReadFile(filename)
	if err != nil {
		return config, err
	}
	err = json.Unmarshal(data, &config)
	return config, err
}

func getInterval(serveTimeMin, serveTimeMax int) (time.Duration, error) {
	randomDuration := serveTimeMin + rand.Intn(serveTimeMax-serveTimeMin)
	return time.Duration(randomDuration) * time.Millisecond, nil
}
