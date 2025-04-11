package main

import (
	"log"
	"wingman/pkg/tts"
)

func main() {
	text := "Hello, this is a test of the text to speech system."

	err := tts.TextToSpeech(text)
	if err != nil {
		log.Fatalf("Error converting text to speech: %v", err)
	}
}
