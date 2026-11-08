package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	fmt.Println("=======================================================")
	fmt.Println(" 🧪 Running Playwright Functional E2E Tests (Go)")
	fmt.Println("=======================================================")

	cmd := exec.Command("go", "test", "-v", "./e2e/...")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("\n❌ E2E tests failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n✅ All Playwright E2E functional tests passed!")
}
