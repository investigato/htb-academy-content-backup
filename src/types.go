package main

import "net/http"
import "time"

type DownloadStatus string

const BASE_FOLDER = "modules"
const (
	StatusPending DownloadStatus = "pending"
	StatusSuccess DownloadStatus = "success"
	StatusFailed  DownloadStatus = "failed"
)

// USER AGENT
type userAgentTransport struct {
	Transport http.RoundTripper
	UserAgent string
}

// TRACKER
type TrackerData struct {
	ModulesDownloaded map[string]ModuleRecord `json:"modules_downloaded"`
	ModulesAvailable  []ModuleData            `json:"modules_available"`
}

// MODULES
type ModuleResponse struct {
	Data ModuleData `json:"data"`
}

type ModuleReponseArray struct {
	Data []ModuleData `json:"data"`
}

type ModuleData struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	WalkthroughID int    `json:"walkthrough_id,omitempty"`
}
type ModuleRecord struct {
	ModuleData
	FriendlyName          string          `json:"friendly_name"`
	Status                DownloadStatus  `json:"status"`
	WalkthroughDownloaded bool            `json:"walkthrough_downloaded"`
	WalkthroughImages     []ImageRecord   `json:"walkthrough_images,omitempty"`
	Sections              []SectionRecord `json:"sections,omitempty"`
	DownloadedAt          time.Time       `json:"downloaded_at,omitempty"`
}

// SECTIONS

type SectionsResponse struct {
	Data []SectionGroup `json:"data"`
}

type SectionGroup struct {
	Group    string    `json:"group"`
	Sections []Section `json:"sections"`
}

type Section struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Page  int    `json:"page"`
}

type SectionContentResponse struct {
	Data SectionContent `json:"data"`
}

type SectionContent struct {
	Content string `json:"content"`
}
type SectionRecord struct {
	ID     int            `json:"id"`
	Title  string         `json:"title"`
	Status DownloadStatus `json:"status"`
	Images []ImageRecord  `json:"images,omitempty"`
	Err    string         `json:"err,omitempty"`
}

// IMAGES

type ImageRef struct {
	OriginalURL string
	// populated after download
	LocalPath string
	Format    string
	Err       error
}

type ImageRecord struct {
	OriginalURL string         `json:"original_url"`
	LocalPath   string         `json:"local_path,omitempty"`
	Format      string         `json:"format,omitempty"`
	Status      DownloadStatus `json:"status"`
	Slot        int            `json:"slot"`
	Err         string         `json:"err,omitempty"`
	AttemptedAt time.Time      `json:"attempted_at"`
}

// WALKTHROUGHS
type WalkthroughResp struct {
	Data Walkthrough `json:"data"`
}

type Walkthrough struct {
	ID           int    `json:"id"`
	ModuleID     int    `json:"module_id"`
	Instructions string `json:"instructions"`
}
