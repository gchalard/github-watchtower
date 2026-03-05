package main

import (
	"fmt"
	"os"
	"time"
)

func Core(event map[string]any) {

	owner, repo := GetEventOwnerAndRepo(event)
	eventType := event["type"].(string)
	eventListeners, err := getEventListeners(owner, repo, eventType)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get ingresses: %v\n", err)
		os.Exit(1)
	}

	triggerURLs, err := GetEventListenerURLs(eventListeners)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get event listener URLs: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("trigger URLs:", triggerURLs)

	body := map[string]any{
		"username": "github-watchtower",
		"foo":      "bar",
		"event":    event,
	}

	eventIds := make([]string, 0, len(triggerURLs))

	for _, url := range triggerURLs {
		eventId, err := TriggerPipeline(url, body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "trigger pipeline: %v\n", err)
			os.Exit(1)
		}
		eventIds = append(eventIds, eventId)
	}

	fmt.Println("event IDs:", eventIds)

	// Give the EventListener a moment to create PipelineRuns
	time.Sleep(2 * time.Second)

	for _, eventId := range eventIds {
		pipelineRuns, err := GetPipelineRuns(eventId)
		var statuses []map[string]any

		if len(pipelineRuns) > 0 {
			statuses, err = GetPipelineRunsStatus(pipelineRuns)
			if err != nil {
				fmt.Fprintf(os.Stderr, "get pipeline runs status: %v\n", err)
				os.Exit(1)
			}
		} else {
			statuses = nil
		}

		for len(pipelineRuns) == 0 && statuses != nil {
			time.Sleep(1 * time.Second)
			fmt.Println("waiting for pipeline runs...")
			pipelineRuns, err = GetPipelineRuns(eventId)
			statuses, err = GetPipelineRunsStatus(pipelineRuns)

			if err != nil {
				fmt.Fprintf(os.Stderr, "get pipeline runs: %v\n", err)
				os.Exit(1)
			}
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "get pipeline runs: %v\n", err)
			os.Exit(1)
		}

		for !PipelineRunsCompleted(statuses) {
			time.Sleep(1 * time.Second)
			fmt.Println("waiting for pipeline runs to complete...")
			DisplayPipelineRunStatus(pipelineRuns)
			pipelineRuns, err = GetPipelineRuns(eventId)
			statuses, err = GetPipelineRunsStatus(pipelineRuns)

			if err != nil {
				fmt.Fprintf(os.Stderr, "get pipeline runs: %v\n", err)
				os.Exit(1)
			}
		}

	}

}
