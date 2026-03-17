package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	modulesToDownload := []ModuleData{}
	options := getArguments()
	fmt.Println("Authenticating with HackTheBox...")
	session := authenticate(options)
	// Load tracker (creates empty one if file doesn't exist)
	tracker := loadTracker()

	if options.retryFailed {
		fmt.Println("Retrying failed image downloads...")
		retryFailedImages(&tracker, session)
		return
	}

	if err := os.MkdirAll(BASE_FOLDER, 0755); err != nil {
		die(err)
	}

	// If no module URL is provided, gotta fetch em all!.
	if options.moduleUrl == "" {
		fmt.Println("No module URL provided. Fetching all accessible modules...")
		modulesToDownload = getAllAvailableModules(session, &tracker)
		// So sad, nothing is available
		if len(modulesToDownload) == 0 {
			fmt.Println("No accessible modules found.")
			os.Exit(1)
		}
	} else {
		// If a specific module URL is provided, then go get that one.
		moduleIDInt := 0
		fmt.Sscanf(options.moduleUrl, "https://academy.hackthebox.com/app/module/%d", &moduleIDInt)
		modulesToDownload = append(modulesToDownload, ModuleData{ID: moduleIDInt})
	}

	for _, module := range modulesToDownload {
		if isModuleDownloaded(module.ID, tracker) {
			fmt.Printf("Skipping already downloaded module: %d\n", module.ID)
			continue
		}

		moduleUrl := fmt.Sprintf("https://academy.hackthebox.com/module/%d", module.ID)
		moduleIDStr := fmt.Sprintf("%d", module.ID)
		fmt.Printf("Processing module: %s\n", moduleUrl)
		fetchedModule, title, sections, content := getModule(moduleUrl, session)

		var sectionImageRecords [][]ImageRecord
		if options.saveImages {
			fmt.Println("Downloading module images...")
			content, sectionImageRecords = getImagesLocally(content, moduleIDStr, session)
		} else {
			content = fixImageUrls(content)
		}

		markdownContent := cleanMarkdown(content)

		err := os.WriteFile(filepath.Join(BASE_FOLDER, moduleFilename(title, moduleIDStr)), []byte(markdownContent), 0666)
		if err != nil {
			die(err)
		}

		// This separates everything out and makes it easier to track if something failed the download process.
		sectionRecords := make([]SectionRecord, len(sections))
		for i, sec := range sections {
			sectionRecords[i] = SectionRecord{
				ID:     sec.ID,
				Title:  sec.Title,
				Status: StatusSuccess,
			}
			if sectionImageRecords != nil && i < len(sectionImageRecords) {
				sectionRecords[i].Images = sectionImageRecords[i]
			}
		}

		var walkthroughImages []ImageRecord
		walkthroughDownloaded := false
		if fetchedModule.WalkthroughID != 0 {
			var walkthroughErr error
			walkthroughImages, walkthroughErr = downloadWalkthrough(fetchedModule, options.saveImages, session)
			if walkthroughErr != nil {
				fmt.Printf("Warning: failed to download walkthrough for module %d: %v\n", fetchedModule.ID, walkthroughErr)
			} else {
				walkthroughDownloaded = true
			}
		}

		updateTracker(&tracker, ModuleRecord{
			ModuleData:            fetchedModule,
			FriendlyName:          title,
			Status:                StatusSuccess,
			WalkthroughDownloaded: walkthroughDownloaded,
			WalkthroughImages:     walkthroughImages,
			Sections:              sectionRecords,
			DownloadedAt:          time.Now(),
		})
		fmt.Printf("Finished downloading module: %s\n", title)
	}
}
