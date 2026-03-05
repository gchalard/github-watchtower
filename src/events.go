package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

func ListEvents(token, owner, repo string) ([]map[string]any, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/events", owner, repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github api %s (status %d): %s", resp.Status, resp.StatusCode, string(body))
	}

	var events []map[string]any
	if err := json.Unmarshal(body, &events); err != nil {
		return nil, fmt.Errorf("decode response: %w (body may be HTML/error page)", err)
	}
	return events, nil
}

func getString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func GetLastEvent(events []map[string]any) (map[string]any, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("no events")
	}

	sort.Slice(events, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, getString(events[i], "created_at"))
		tj, _ := time.Parse(time.RFC3339, getString(events[j], "created_at"))
		return ti.After(tj)
	})

	return events[0], nil
}

func eventID(ev map[string]any) string {
	if v, ok := ev["id"].(string); ok {
		return v
	}
	// fallback for numeric id from JSON
	if v, ok := ev["id"].(float64); ok {
		return fmt.Sprintf("%.0f", v)
	}
	return ""
}

// EventsInFirstNotInSecond returns events that are in first but not in second (by id).
func EventsInFirstNotInSecond(first, second []map[string]any) []map[string]any {
	ids := make(map[string]bool)
	for _, ev := range second {
		ids[eventID(ev)] = true
	}
	var out []map[string]any
	for _, ev := range first {
		if !ids[eventID(ev)] {
			out = append(out, ev)
		}
	}
	return out
}

func NewEvents(events []map[string]any, initDatetime time.Time) bool {
	lastEvent, err := GetLastEvent(events)
	if err != nil {
		return false
	}
	lastEventDatetime, err := time.Parse(time.RFC3339, getString(lastEvent, "created_at"))
	if err != nil {
		return false
	}

	return lastEventDatetime.After(initDatetime)

}

func WaitForNewEvents(token, owner, repo string, initDatetime time.Time) ([]map[string]any, error) {
	events, err := ListEvents(token, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}

	initialEvents := make([]map[string]any, len(events))
	copy(initialEvents, events)

	for !NewEvents(events, initDatetime) {
		fmt.Println("no new events, waiting for 1 second...")
		time.Sleep(15 * time.Second)
		events, err = ListEvents(token, owner, repo)
		if err != nil {
			return nil, fmt.Errorf("list events: %w", err)
		}
	}

	newEvents := EventsInFirstNotInSecond(events, initialEvents)
	newEventsIds := make([]string, 0, len(newEvents))
	for _, event := range newEvents {
		newEventsIds = append(newEventsIds, eventID(event))
	}

	fmt.Println("new events IDs:", newEventsIds)

	return newEvents, nil
}

func GetEventOwnerAndRepo(event map[string]any) (string, string) {
	// GitHub Events API: repo is at top level, e.g. {"repo": {"name": "owner/repo", ...}}
	repo, ok := event["repo"].(map[string]any)
	if !ok {
		return "", ""
	}
	fullName, ok := repo["name"].(string)
	if !ok {
		return "", ""
	}
	parts := strings.Split(fullName, "/")
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}
