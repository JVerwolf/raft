package main

import (
    "time"
    "fmt"
)

type ticker struct {
    period time.Duration
    ticker time.Ticker
}

func createTicker(period time.Duration) *ticker {
    return &ticker{period, *time.NewTicker(period)}
}

func (t *ticker) resetTicker() {
    t.ticker = *time.NewTicker(t.period)
}

type server struct {
    doneChan chan bool
    tickerA  ticker
    tickerB  ticker
}

func (s *server) listener() {
    start := time.Now()
    for {
        select {
        case <-s.tickerA.ticker.C:
            elapsed := time.Since(start)
            fmt.Println("Elapsed: ", elapsed, " Ticker A")
        case <-s.tickerB.ticker.C:
            s.tickerA.resetTicker()
            elapsed := time.Since(start)
            fmt.Println("Elapsed: ", elapsed, " Ticker B - Going to reset ticker A")
            s.tickerA.resetTicker()
        }
    }
    s.doneChan <- true
}

func main() {
    doneChan := make(chan bool)
    tickerA := createTicker(3 * time.Second)
    tickerB := createTicker(1 * time.Second)
    s := &server{doneChan, *tickerA, *tickerB}
    go s.listener()
    <-doneChan
}
