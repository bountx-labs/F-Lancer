package scraper

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"time"
)

type RSSFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel Channel  `xml:"channel"`
}

type Channel struct {
	Title string `xml:"title"`
	Items []Item `xml:"item"`
}

type Item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Budget      string `xml:"budget"`
	Category    string `xml:"category"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
}

type Job struct {
	Title       string
	Link        string
	Description string
	Budget      string
	Category    string
	PubDate     time.Time
	GUID        string
	FeedSource  string
}

func FetchFeed(url string) ([]Job, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
	}

	var feed RSSFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("parse %s: %w", url, err)
	}

	var jobs []Job
	for _, item := range feed.Channel.Items {
		pubDate, _ := parseDate(item.PubDate)
		jobs = append(jobs, Job{
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
			Budget:      item.Budget,
			Category:    item.Category,
			PubDate:     pubDate,
			GUID:        item.GUID,
			FeedSource:  url,
		})
	}

	return jobs, nil
}

func parseDate(s string) (time.Time, error) {
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"Mon, 02 Jan 2006 15:04:05 -0700",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse date: %s", s)
}