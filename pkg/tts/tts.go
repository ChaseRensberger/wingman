package tts

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// TextToSpeech converts the given text to speech using the Python script
func TextToSpeech(text string) error {
	// Get the absolute path to the tts.py script
	_, currentFile, _, _ := runtime.Caller(0)
	scriptPath := filepath.Join(filepath.Dir(currentFile), "tts.py")

	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(currentFile)))

	pythonPath := filepath.Join(projectRoot, "venv", "bin", "python3")

	cmd := exec.Command(pythonPath, scriptPath, text)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
