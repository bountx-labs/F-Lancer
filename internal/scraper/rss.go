package scraper

import (
	"encoding/xml"
	"fmt"
	"log"
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

// FetchFeed retrieves and parses an RSS feed, retrying transient network and
// 5xx/429 failures with exponential backoff.
func FetchFeed(url string, timeout time.Duration) ([]Job, error) {
	client := &http.Client{Timeout: timeout}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second)
		}

		resp, err := client.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("fetch %s: %w", url, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			status := resp.StatusCode
			resp.Body.Close()
			lastErr = fmt.Errorf("fetch %s: HTTP %d", url, status)
			if status >= http.StatusInternalServerError || status == http.StatusTooManyRequests {
				continue
			}
			break
		}

		var feed RSSFeed
		if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("parse %s: %w", url, err)
		}
		resp.Body.Close()

		var jobs []Job
		for _, item := range feed.Channel.Items {
			pubDate, err := parseDate(item.PubDate)
			if err != nil {
				log.Printf("date parse failed for %s: %v", url, err)
				pubDate = time.Now()
			}
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

	return nil, lastErr
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