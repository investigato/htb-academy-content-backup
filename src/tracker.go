package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func loadTracker() TrackerData {
	var trackerData TrackerData
	content, err := os.ReadFile("tracker.json")
	if err != nil {
		return trackerData
	}
	if err := json.Unmarshal(content, &trackerData); err != nil {
		die(err)
	}
	return trackerData
}

func saveTracker(data TrackerData) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp := "tracker.json.tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, "tracker.json") // do or do not, there is no try
}

func isModuleDownloaded(moduleID int, tracker TrackerData) bool {
	if tracker.ModulesDownloaded == nil {
		return false
	}
	rec, ok := tracker.ModulesDownloaded[fmt.Sprintf("%d", moduleID)]
	return ok && rec.Status == StatusSuccess
}

func updateTracker(tracker *TrackerData, module ModuleRecord) {
	if tracker.ModulesDownloaded == nil {
		tracker.ModulesDownloaded = make(map[string]ModuleRecord)
	}
	tracker.ModulesDownloaded[fmt.Sprintf("%d", module.ID)] = module
	if err := saveTracker(*tracker); err != nil {
		die(err)
	}
}
