package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/publicsuffix"
)

const cdnBase = "https://cdn.services-k8s.prod.aws.htb.systems"
const academyBase = "https://academy.hackthebox.com"

func (ua *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", ua.UserAgent)
	}
	return ua.Transport.RoundTrip(req)
}

func createHttpClient(options Args) *http.Client {
	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if options.cookies != "" {
		addCookiesToJar(jar, options.cookies)
	}

	var transport http.RoundTripper = http.DefaultTransport
	if options.proxy {
		proxyAddr, _ := url.Parse("http://127.0.0.1:8080")
		transport = &http.Transport{
			Proxy:           http.ProxyURL(proxyAddr),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	transport = &userAgentTransport{
		Transport: transport,
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36",
	}

	return &http.Client{
		Jar:       jar,
		Transport: transport,
	}
}

func addCookiesToJar(jar *cookiejar.Jar, cookies string) {
	cookiePairs := strings.Split(cookies, ";")
	cookieList := []*http.Cookie{}
	// if idiots like me just paste the htb_academy_session cookie VALUE, add htb_academy_session= to the front so it gets parsed correctly.
	if len(cookiePairs) == 1 && !strings.Contains(cookiePairs[0], "=") {
		cookiePairs[0] = "htb_academy_session=" + cookiePairs[0]
	}
	for _, pair := range cookiePairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			cookieList = append(cookieList, &http.Cookie{
				Name:  strings.TrimSpace(parts[0]),
				Value: strings.TrimSpace(parts[1]),
			})
		}
	}

	u, _ := url.Parse(academyBase)
	jar.SetCookies(u, cookieList)
}

func authenticate(options Args) *http.Client {
	client := createHttpClient(options)
	resp, err := client.Get(academyBase + "/api/v2/modules?state=owned")
	if err != nil {
		die(fmt.Errorf("authentication failed: %v", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		die(fmt.Errorf("authentication failed with status %d: %s", resp.StatusCode, string(body)))
	}

	return client
}

func getAllAvailableModules(client *http.Client, tracker *TrackerData) []ModuleData {
	apiUrl := "https://academy.hackthebox.com/api/v2/modules"
	stateValues := []string{"owned", "in_progress"}
	seen := make(map[int]bool)
	var allModules []ModuleData

	for _, state := range stateValues {
		req, err := http.NewRequest("GET", apiUrl+"?state="+state, nil)
		if err != nil {
			die(err)
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("Referer", "https://academy.hackthebox.com/app/library/modules")
		resp, err := client.Do(req)
		if err != nil {
			die(err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			die(err)
		}

		if resp.StatusCode != http.StatusOK {
			fmt.Printf("Failed to fetch modules. Status: %d\n", resp.StatusCode)
			fmt.Println("Response:", string(body))
			os.Exit(1)
		}

		var modulesResp ModuleResponseArray
		if err := json.Unmarshal(body, &modulesResp); err != nil {
			die(err)
		}

		for _, module := range modulesResp.Data {
			if !seen[module.ID] {
				seen[module.ID] = true
				allModules = append(allModules, module)
			}
		}
	}

	// Replace cached list with current API state so new modules appear on next run
	tracker.ModulesAvailable = allModules
	saveTracker(*tracker)

	fmt.Printf("Found %d available modules.\n", len(allModules))
	return allModules
}

func getModule(moduleUrl string, client *http.Client) (ModuleData, string, []Section, []string) {
	moduleID := extractModuleID(moduleUrl)
	refererUrl := normalizeModuleUrl(moduleUrl)

	moduleData, moduleTitle := getModuleMetadata(moduleID, refererUrl, client)

	sections := getModuleSections(moduleID, refererUrl, client)

	var pagesContent []string
	for _, section := range sections {
		content := getSectionContent(moduleID, section.ID, refererUrl, client)
		pagesContent = append(pagesContent, content)
	}

	return moduleData, moduleTitle, sections, pagesContent
}

func extractModuleID(moduleUrl string) string {
	// Parse URL like: https://academy.hackthebox.com/module/163/section/1546
	// or: https://academy.hackthebox.com/app/module/163/section/1546
	parts := strings.Split(moduleUrl, "/")
	for i, part := range parts {
		if part == "module" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	fmt.Println("Could not extract module ID from URL:", moduleUrl)
	os.Exit(1)
	return ""
}

func normalizeModuleUrl(moduleUrl string) string {
	// Ensure URL uses /app/module/ format
	// Convert: https://academy.hackthebox.com/module/163/...
	// To: https://academy.hackthebox.com/app/module/163/...
	if strings.Contains(moduleUrl, "/app/module/") {
		return moduleUrl
	}
	return strings.Replace(moduleUrl, "/module/", "/app/module/", 1)
}

func getModuleMetadata(moduleID string, refererUrl string, client *http.Client) (ModuleData, string) {
	apiUrl := fmt.Sprintf("https://academy.hackthebox.com/api/v2/modules/%s", moduleID)

	req, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		die(err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", refererUrl)

	resp, err := client.Do(req)
	if err != nil {
		die(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		die(err)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Failed to fetch module metadata. Status: %d\n", resp.StatusCode)
		fmt.Println("Response:", string(body))
		os.Exit(1)
	}

	var moduleResp ModuleResponse
	if err := json.Unmarshal(body, &moduleResp); err != nil {
		die(err)
	}

	return moduleResp.Data, moduleResp.Data.Name
}

func getModuleSections(moduleID string, refererUrl string, client *http.Client) []Section {
	apiUrl := fmt.Sprintf(academyBase+"/api/v3/modules/%s/sections", moduleID)

	req, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		die(err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", refererUrl)

	resp, err := client.Do(req)
	if err != nil {
		die(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		die(err)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Failed to fetch module sections. Status: %d\n", resp.StatusCode)
		fmt.Println("Response:", string(body))
		os.Exit(1)
	}

	var sectionsResp SectionsResponse
	if err := json.Unmarshal(body, &sectionsResp); err != nil {
		die(err)
	}

	// Flatten all sections from all groups and sort by page number
	var allSections []Section
	for _, group := range sectionsResp.Data {
		allSections = append(allSections, group.Sections...)
	}

	// Sort sections by page number
	for i := 0; i < len(allSections); i++ {
		for j := i + 1; j < len(allSections); j++ {
			if allSections[i].Page > allSections[j].Page {
				allSections[i], allSections[j] = allSections[j], allSections[i]
			}
		}
	}

	return allSections
}

func getSectionContent(moduleID string, sectionID int, refererUrl string, client *http.Client) string {
	apiUrl := fmt.Sprintf(academyBase+"/api/v2/modules/%s/sections/%d", moduleID, sectionID)

	req, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		die(err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", refererUrl)

	resp, err := client.Do(req)
	if err != nil {
		die(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		die(err)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Failed to fetch section %d. Status: %d\n", sectionID, resp.StatusCode)
		fmt.Println("Response:", string(body))
		os.Exit(1)
	}

	var contentResp SectionContentResponse
	if err := json.Unmarshal(body, &contentResp); err != nil {
		die(err)
	}

	// The content is already in markdown format with \r\n line endings
	// Normalize line endings to \n
	content := strings.ReplaceAll(contentResp.Data.Content, "\r\n", "\n")

	return content
}

func downloadWalkthrough(module ModuleData, localImages bool, client *http.Client) ([]ImageRecord, error) {
	if module.WalkthroughID == 0 {
		return nil, nil
	}

	apiUrl := fmt.Sprintf("https://academy.hackthebox.com/api/v2/walkthroughs/%d", module.WalkthroughID)

	req, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", fmt.Sprintf("https://academy.hackthebox.com/app/module/%d", module.ID))

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch walkthrough for module %d: status %d: %s", module.ID, resp.StatusCode, string(body))
	}
	var walkthroughResp WalkthroughResp
	if err := json.Unmarshal(body, &walkthroughResp); err != nil {
		return nil, err
	}

	moduleIDStr := fmt.Sprintf("%d", module.ID)
	walkthroughFileName := walkthroughFilename(module.Name, moduleIDStr)

	var imgRecords []ImageRecord
	walkthroughInstructionsRaw := []string{walkthroughResp.Data.Instructions}
	if localImages {
		fmt.Println("Downloading walkthrough images...")
		var perSection [][]ImageRecord
		walkthroughInstructionsRaw, perSection = getImagesLocally(walkthroughInstructionsRaw, moduleIDStr+"-walkthrough", client)
		if len(perSection) > 0 {
			imgRecords = perSection[0]
		}
	} else {
		walkthroughInstructionsRaw = fixImageUrls(walkthroughInstructionsRaw)
	}
	// this is just to cleanup the markdown a bit, it also removes the invalid `-session` that HTB appends to the fenced code blocks.
	walkthroughInstructions := cleanMarkdown(walkthroughInstructionsRaw)

	return imgRecords, os.WriteFile(filepath.Join(BASE_FOLDER, walkthroughFileName), []byte(walkthroughInstructions), 0666)
}

func die(err error) {
	fmt.Println(err)
	os.Exit(1)
}
