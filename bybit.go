package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const (
	bybitBaseURL = "https://api.bybit.com/v5/market/tickers?category=spot"
)

// TickerResponse represents the Bybit API response
type TickerResponse struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
	Result  struct {
		List []struct {
			Symbol       string `json:"symbol"`
			LastPrice    string `json:"lastPrice"`
			Price24hPcnt string `json:"price24hPcnt"`
			Volume24h    string `json:"volume24h"`
		} `json:"list"`
	} `json:"result"`
}

var lastRequestTime time.Time
var minInterval = 2 * time.Second // Limit: 1 request per 2 seconds

// fetchTickers fetches all tickers from Bybit with rate limiting
func fetchTickers() (*TickerResponse, error) {
	now := time.Now()
	if !lastRequestTime.IsZero() && now.Sub(lastRequestTime) < minInterval {
		wait := minInterval - now.Sub(lastRequestTime)
		log.Printf("Rate limit: waiting %v", wait)
		time.Sleep(wait)
	}
	lastRequestTime = time.Now()

	req, err := http.NewRequest("GET", bybitBaseURL, nil)
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Referer", "https://bybit.com/")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("HTTP request error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Read body error: %v", err)
		return nil, err
	}

	log.Printf("Bybit tickers response: %s", string(body)[:500]) // Log first 500 chars

	var tickers TickerResponse
	if err := json.Unmarshal(body, &tickers); err != nil {
		log.Printf("JSON unmarshal error: %v", err)
		return nil, err
	}

	if tickers.RetCode != 0 {
		log.Printf("API error: %s", tickers.RetMsg)
		return nil, fmt.Errorf("API error: %s", tickers.RetMsg)
	}

	return &tickers, nil
}
