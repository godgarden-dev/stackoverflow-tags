package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gocarina/gocsv"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/pkg/errors"
)

const (
	Endpoint        string = "https://api.stackexchange.com/2.2"
	DefaultPageSize int    = 100
)

type Client struct {
	URL        *url.URL
	HTTPClient *http.Client
}

type TagResponse struct {
	Items          []Tag `json:"items"`
	HasMore        bool  `json:"has_more"`
	QuotaMax       int   `json:"quota_max"`
	QuotaRemaining int   `json:"quota_remaining"`
}

type Tag struct {
	HasSynonyms     bool   `json:"has_synonyms" csv:"has_synonyms"`
	IsModeratorOnly bool   `json:"is_moderator_only" csv:"is_moderator_only"`
	IsRequired      bool   `json:"is_required" csv:"is_required"`
	Count           int    `json:"count" csv:"count"`
	Name            string `json:"name" csv:"name"`
}

func main() {

	log.Println("INFO:START")

	client, err := NewClient(Endpoint)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	tags, err := client.listTags(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(tags)
	if err := output(tags); err != nil {
		log.Fatal(err)
	}

	log.Println("INFO:END")
}

func NewClient(urlStr string) (*Client, error) {
	parsedURL, err := url.ParseRequestURI(urlStr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse url: %s", urlStr)
	}
	return &Client{URL: parsedURL, HTTPClient: http.DefaultClient}, nil
}

func (c *Client) newRequest(ctx context.Context, method, spath string, body io.Reader) (*http.Request, error) {
	u := *c.URL
	u.Path = path.Join(c.URL.Path, spath)

	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return nil, err
	}

	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

func (c *Client) listTags(ctx context.Context) ([]Tag, error) {
	var items []Tag
	var page int = 1

	req, err := c.newRequest(ctx, "GET", "/tags", nil)
	if err != nil {
		return nil, err
	}
	q := url.Values{
		"page":     []string{strconv.Itoa(page)},
		"per_page": []string{strconv.Itoa(DefaultPageSize)},
		"order":    []string{"desc"},
		"sort":     []string{"popular"},
		"site":     []string{"stackoverflow"},
	}
	req.URL.RawQuery = q.Encode()

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != 200 {
		log.Printf("break!! status:%d", res.StatusCode)
		return nil, nil
	}

	var tagResp *TagResponse
	if err := decodeBody(res, &tagResp); err != nil {
		return nil, err
	}
	items = append(items, tagResp.Items...)

	for tagResp.HasMore {
		req, err := c.newRequest(ctx, "GET", "/tags", nil)
		if err != nil {
			return nil, err
		}

		q.Set("page", strconv.Itoa(page+1))
		req.URL.RawQuery = q.Encode()

		res, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}

		if res.StatusCode != 200 {
			log.Printf("break!! status:%d", res.StatusCode)
			break
		}

		var tags TagResponse
		if err := decodeBody(res, &tags); err != nil {
			return nil, err
		}

		// Request間隔の調整
		time.Sleep(time.Second * 60)
		items = append(items, tagResp.Items...)
	}

	return items, nil
}

func decodeBody(resp *http.Response, out interface{}) error {
	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)
	return decoder.Decode(out)
}

func output(tags []Tag) error {
	file, err := os.OpenFile("/tmp/stackoverflow_tags.csv", os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := file.Truncate(0); err != nil {
		log.Fatal(err)
	}

	if err := gocsv.MarshalFile(&tags, file); err != nil {
		log.Fatal(err)
	}

	return nil
}
