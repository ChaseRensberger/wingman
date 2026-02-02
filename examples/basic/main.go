package main

import (
	"wingman/agent"
	"wingman/provider/anthropic"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load(".env.local")

	p := anthropic.New()
}
