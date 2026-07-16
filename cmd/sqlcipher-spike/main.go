package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/Cail-Gainey/MineOps/internal/infrastructure/sqlcipher"
)

func main() {
	result, err := sqlcipher.RunSpike(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	if !result.Passed() {
		log.Fatal("SQLCipher spike did not satisfy every gate")
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		log.Fatal(err)
	}
}
