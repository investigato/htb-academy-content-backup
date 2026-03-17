package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	gmarkdown "github.com/teekennedy/goldmark-markdown"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"golang.org/x/net/html"
)

type imageRef struct {
	src string
	alt string
}

type imageTransformer struct {
	moduleID       string
	counter        *int
	download       bool
	client         *http.Client
	sectionRecords []ImageRecord
}

func newMarkdownProcessor(t *imageTransformer) goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithASTTransformers(util.Prioritized(t, 100)),
		),
		goldmark.WithRenderer(gmarkdown.NewRenderer()),
	)
}

func (t *imageTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	src := reader.Source()

	// Collect everything first. It's a terrible idea to chew gum while walking.
	var mdImages []*ast.Image
	var rawHTMLs []*ast.RawHTML
	var htmlBlocks []*ast.HTMLBlock

	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Image:
			mdImages = append(mdImages, node)

		case *ast.RawHTML:
			for i := 0; i < node.Segments.Len(); i++ {
				segment := node.Segments.At(i)
				if strings.Contains(strings.ToLower(string(segment.Value(src))), "<img") {
					rawHTMLs = append(rawHTMLs, node)
					break
				}
			}

		case *ast.HTMLBlock:
			lines := node.Lines()
			if lines.Len() == 0 {
				break
			}
			for i := 0; i < lines.Len(); i++ {
				segment := lines.At(i)
				if strings.Contains(strings.ToLower(string(segment.Value(src))), "<img") {
					htmlBlocks = append(htmlBlocks, node)
					break
				}
			}
		}
		return ast.WalkContinue, nil
	})

	for _, node := range mdImages {
		node.Destination = []byte(t.processImage(string(node.Destination)))
	}

	for _, node := range rawHTMLs {
		var fragment strings.Builder
		for i := 0; i < node.Segments.Len(); i++ {
			segment := node.Segments.At(i)
			fragment.Write(segment.Value(src))
		}
		refs := parseHTMLImageRefs(fragment.String())
		if len(refs) == 0 {
			continue
		}
		parent := node.Parent()
		for _, ref := range refs {
			parent.InsertBefore(parent, node, buildImageNode(t.processImage(ref.src), ref.alt))
		}
		parent.RemoveChild(parent, node)
	}

	for _, node := range htmlBlocks {
		var buf strings.Builder
		lines := node.Lines()
		for i := 0; i < lines.Len(); i++ {
			segment := lines.At(i)
			buf.Write(segment.Value(src))
		}
		refs := parseHTMLImageRefs(buf.String())
		if len(refs) == 0 {
			continue
		}
		parent := node.Parent()
		for _, ref := range refs {
			para := ast.NewParagraph()
			para.AppendChild(para, buildImageNode(t.processImage(ref.src), ref.alt))
			parent.InsertBefore(parent, node, para)
		}
		parent.RemoveChild(parent, node)
	}
}

func (t *imageTransformer) processImage(src string) string {
	url := resolveURL(src)
	if !t.download {
		return url
	}

	*t.counter++
	record := ImageRecord{
		OriginalURL: url,
		Slot:        *t.counter,
		AttemptedAt: time.Now(),
	}

	data, _, err := fetchImageBytes(url, t.client)
	if err != nil {
		record.Status = StatusFailed
		record.Err = err.Error()
		t.sectionRecords = append(t.sectionRecords, record)
		fmt.Printf("Warning: failed to fetch image %s: %v\n", url, err)
		return url
	}

	localPath, format, err := writeImage(data, t.moduleID, *t.counter)
	if err != nil {
		record.Status = StatusFailed
		record.Err = err.Error()
		t.sectionRecords = append(t.sectionRecords, record)
		fmt.Printf("Warning: skipping image %s: %v\n", url, err)
		return url
	}

	record.LocalPath = localPath
	record.Format = format
	record.Status = StatusSuccess
	t.sectionRecords = append(t.sectionRecords, record)
	return localPath
}

func buildImageNode(dst, alt string) *ast.Image {
	link := ast.NewLink()
	link.Destination = []byte(dst)
	img := ast.NewImage(link)
	if alt != "" {
		img.AppendChild(img, ast.NewString([]byte(alt)))
	}
	return img
}

func parseHTMLImageRefs(fragment string) []imageRef {
	var refs []imageRef
	z := html.NewTokenizer(strings.NewReader(fragment))
	for {
		tokenType := z.Next()
		if tokenType == html.ErrorToken {
			break
		}
		if tokenType != html.StartTagToken && tokenType != html.SelfClosingTagToken {
			continue
		}
		token := z.Token()
		if token.Data != "img" {
			continue
		}
		var src, alt string
		for _, attr := range token.Attr {
			switch attr.Key {
			case "src":
				src = attr.Val
			case "alt":
				alt = attr.Val
			}
		}
		if src != "" {
			refs = append(refs, imageRef{src: src, alt: alt})
		}
	}
	return refs
}

func resolveURL(src string) string {
	if strings.HasPrefix(src, "http") {
		return src
	}
	if strings.HasPrefix(src, "/content/") {
		return cdnBase + src
	}
	return academyBase + src
}

func fetchImageBytes(url string, client *http.Client) ([]byte, string, error) {
	data, finalURL, err := doGet(url, client)
	if err == nil {
		return data, finalURL, nil
	}
	// CDN fallback.
	cdnURL := cdnBase + strings.TrimPrefix(url, academyBase)
	if cdnURL == url {
		return nil, url, err
	}
	data, finalURL, cdnErr := doGet(cdnURL, client)
	if cdnErr != nil {
		return nil, url, fmt.Errorf("fetch failed: academy: %w | cdn: %v", err, cdnErr)
	}
	return data, finalURL, nil
}

func doGet(url string, client *http.Client) ([]byte, string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, url, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, url, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	return data, url, err
}

func validateImageBytes(data []byte) (string, error) {
	ct := http.DetectContentType(data)
	switch ct {
	case "image/png":
		return ".png", nil
	case "image/jpeg":
		return ".jpg", nil
	case "image/gif":
		return ".gif", nil
	case "image/webp":
		return ".webp", nil
	default:
		return "", fmt.Errorf("unrecognised image content (detected %q)", ct)
	}
}

// writeImage validates, writes, and returns (localPath, format, error).
func writeImage(data []byte, moduleID string, counter int) (string, string, error) {
	ext, err := validateImageBytes(data)
	if err != nil {
		return "", "", err
	}
	path := fmt.Sprintf("%s/images/module-%s-%03d%s", BASE_FOLDER, moduleID, counter, ext)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, strings.TrimPrefix(ext, "."), nil
}

func processImages(sections []string, moduleID string, download bool, client *http.Client) ([]string, [][]ImageRecord) {
	counter := 0
	t := &imageTransformer{moduleID: moduleID, download: download, client: client, counter: &counter}
	md := newMarkdownProcessor(t)

	result := make([]string, len(sections))
	allRecords := make([][]ImageRecord, len(sections))
	for i, section := range sections {
		t.sectionRecords = nil // reset for each section
		var buf bytes.Buffer
		if err := md.Convert([]byte(section), &buf); err != nil {
			result[i] = section
		} else {
			result[i] = buf.String()
		}
		allRecords[i] = t.sectionRecords
	}
	return result, allRecords
}

func fixImageUrls(sections []string) []string {
	result, _ := processImages(sections, "", false, nil)
	return result
}

func getImagesLocally(sections []string, moduleID string, client *http.Client) ([]string, [][]ImageRecord) {
	imgDir := BASE_FOLDER + "/images"
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		die(err)
	}
	return processImages(sections, moduleID, true, client)
}

// this frequently hits the rate limiter on HackTheBox, so this will retry any failed images.
func retryFailedImages(tracker *TrackerData, client *http.Client) {
	for id, module := range tracker.ModulesDownloaded {
		moduleIDStr := fmt.Sprintf("%d", module.ID)
		changed := false

		mdPath := filepath.Join(BASE_FOLDER, moduleFilename(module.Name, moduleIDStr))
		for si := range module.Sections {
			for ii := range module.Sections[si].Images {
				img := &module.Sections[si].Images[ii]
				if img.Status != StatusFailed {
					continue
				}
				if newPath, err := retryImage(img, moduleIDStr, client); err == nil {
					patchMarkdownImage(mdPath, img.OriginalURL, newPath)
				}
				changed = true
			}
		}

		wtPath := filepath.Join(BASE_FOLDER, walkthroughFilename(module.Name, moduleIDStr))
		for ii := range module.WalkthroughImages {
			img := &module.WalkthroughImages[ii]
			if img.Status != StatusFailed {
				continue
			}
			if newPath, err := retryImage(img, moduleIDStr+"-walkthrough", client); err == nil {
				patchMarkdownImage(wtPath, img.OriginalURL, newPath)
			}
			changed = true
		}

		if changed {
			tracker.ModulesDownloaded[id] = module
			if err := saveTracker(*tracker); err != nil {
				fmt.Printf("Warning: failed to save tracker after retrying module %s: %v\n", id, err)
			}
		}
	}
}

func retryImage(img *ImageRecord, moduleIDStr string, client *http.Client) (string, error) {
	fmt.Printf("Retrying image %s\n", img.OriginalURL)
	img.AttemptedAt = time.Now()

	data, _, err := fetchImageBytes(img.OriginalURL, client)
	if err != nil {
		img.Err = err.Error()
		return "", err
	}

	localPath, format, err := writeImage(data, moduleIDStr, img.Slot)
	if err != nil {
		img.Err = err.Error()
		return "", err
	}

	img.LocalPath = localPath
	img.Format = format
	img.Status = StatusSuccess
	img.Err = ""
	return localPath, nil
}

func patchMarkdownImage(mdPath, originalURL, localPath string) {
	content, err := os.ReadFile(mdPath)
	if err != nil {
		fmt.Printf("Warning: could not read %s to patch image reference: %v\n", mdPath, err)
		return
	}
	updated := strings.ReplaceAll(string(content), "]("+originalURL+")", "]("+localPath+")")
	if err := os.WriteFile(mdPath, []byte(updated), 0666); err != nil {
		fmt.Printf("Warning: could not write %s after patching image reference: %v\n", mdPath, err)
	}
}
