package main

import (
	"log"

	"github.com/DANG-PH/game-service-go/internal/app"
	"github.com/DANG-PH/game-service-go/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	a := app.New(cfg)
	if err := a.Run(); err != nil {
		log.Fatalf("run app: %v", err)
	}
}
