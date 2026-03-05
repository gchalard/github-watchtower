package main

import (
	"fmt"
	"os"
	"sync"
	"time"
)

func main() {
	installationID := os.Getenv("INSTALLATION_ID")
	appId := os.Getenv("APP_ID")
	privateKey := os.Getenv("PRIVATE_KEY")
	owner := os.Getenv("OWNER")
	repo := os.Getenv("REPO")

	initDatetime := time.Now()
	datetime := initDatetime
	var wg sync.WaitGroup

	for true {
		token, err := GetInstallationAccessToken(appId, installationID, privateKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "get installation token: %v\n", err)
			os.Exit(1)
		}

		events, err := WaitForNewEvents(token, owner, repo, datetime)
		if err != nil {
			fmt.Fprintf(os.Stderr, "wait for new events: %v\n", err)
			os.Exit(1)
		}

		for _, event := range events {
			event := event
			wg.Go(func() {
				Core(event)
			})
		}

		datetime = time.Now()
	}

	wg.Wait()
}
