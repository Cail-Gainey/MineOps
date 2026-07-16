package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/Cail-Gainey/MineOps/internal/infrastructure/minecraftspark"
)

func main() {
	result, err := minecraftspark.RunSpike()
	if err != nil {
		log.Fatal(err)
	}
	if !result.Passed() {
		log.Fatal("Minecraft spark adapter spike did not satisfy every gate")
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		log.Fatal(err)
	}
}
