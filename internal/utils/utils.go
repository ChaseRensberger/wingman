package utils

import (
	"fmt"
)

const (
	colorReset  = "\033[0m"
	colorBlue   = "\033[34m"
	colorRed    = "\033[31m"
	colorOrange = "\033[33m"
	colorBold   = "\033[1m"
)

func UserPrint(text string) {
	fmt.Printf("%s%sUser:%s %s%s\n", colorBold, colorBlue, colorReset, colorBlue, text)
	fmt.Print(colorReset)
}

func AgentPrint(text string) {
	fmt.Printf("%s%sAgent:%s %s%s\n", colorBold, colorRed, colorReset, colorRed, text)
	fmt.Print(colorReset)
}

func ToolPrint(text string) {
	fmt.Printf("%s%sTool:%s %s%s\n", colorBold, colorOrange, colorReset, colorOrange, text)
	fmt.Print(colorReset)
}
