package handlers

import (
	"fmt"
	"log"
	"sort"
	"time"
)

// TickerResponse представляє відповідь API Bybit для тікерів
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

func ParseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

func AlertWorker(alerts map[string]float64, fetchTickers func() (*TickerResponse, error)) {
	for {
		time.Sleep(30 * time.Second)
		if len(alerts) == 0 {
			continue
		}
		tickers, err := fetchTickers()
		if err != nil {
			log.Printf("Помилка отримання даних для сигналу: %v", err)
			continue
		}
		for symbol, target := range alerts {
			for _, t := range tickers.Result.List {
				if t.Symbol == symbol {
					price, _ := ParseFloat(t.LastPrice)
					if price >= target {
						log.Printf("СИГНАЛ: %s ціна %.2f >= %.2f", symbol, price, target)
						delete(alerts, symbol)
					}
				}
			}
		}
	}
}

func TgHandlePrice(symbol string, fetchTickers func() (*TickerResponse, error)) string {
	tickers, err := fetchTickers()
	if err != nil {
		return "Помилка отримання ціни: " + err.Error()
	}
	for _, t := range tickers.Result.List {
		if t.Symbol == symbol {
			return fmt.Sprintf("%s price: %s", symbol, t.LastPrice)
		}
	}
	return "Символ не знайдено."
}

func TgHandleChange(symbol string, fetchTickers func() (*TickerResponse, error)) string {
	tickers, err := fetchTickers()
	if err != nil {
		return "Помилка отримання змін: " + err.Error()
	}
	for _, t := range tickers.Result.List {
		if t.Symbol == symbol {
			return fmt.Sprintf("%s змін за 24h: %s%%", symbol, t.Price24hPcnt)
		}
	}
	return "Символ не знайдено."
}

func TgHandleGainers(fetchTickers func() (*TickerResponse, error)) string {
	tickers, err := fetchTickers()
	if err != nil {
		return "Помилка отримання лідерів: " + err.Error()
	}
	type pair struct {
		Symbol string
		Change float64
	}
	var pairs []pair
	for _, t := range tickers.Result.List {
		c, _ := ParseFloat(t.Price24hPcnt)
		pairs = append(pairs, pair{t.Symbol, c})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Change > pairs[j].Change })
	res := "Топ-5 лідерів за зростанням:\n"
	for i := 0; i < 5 && i < len(pairs); i++ {
		res += fmt.Sprintf("%s: %.2f%%\n", pairs[i].Symbol, pairs[i].Change)
	}
	return res
}

func TgHandleLosers(fetchTickers func() (*TickerResponse, error)) string {
	tickers, err := fetchTickers()
	if err != nil {
		return "Помилка отримання аутсайдерів: " + err.Error()
	}
	type pair struct {
		Symbol string
		Change float64
	}
	var pairs []pair
	for _, t := range tickers.Result.List {
		c, _ := ParseFloat(t.Price24hPcnt)
		pairs = append(pairs, pair{t.Symbol, c})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Change < pairs[j].Change })
	res := "Топ-5 аутсайдерів за падінням:\n"
	for i := 0; i < 5 && i < len(pairs); i++ {
		res += fmt.Sprintf("%s: %.2f%%\n", pairs[i].Symbol, pairs[i].Change)
	}
	return res
}
